package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"

	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
)

const attemptTimeout = 20 * time.Second

type Verifier struct {
	cfg config.Telegram
	log *slog.Logger

	dcList dcs.List
}

func NewVerifier(cfg config.Telegram, log *slog.Logger) *Verifier {
	return &Verifier{
		cfg:    cfg,
		log:    log,
		dcList: dcs.Prod(),
	}
}

func (v *Verifier) Verify(ctx context.Context, keys []domain.DatacenterKey) (*domain.ExtractedData, error) {
	const fn = "infra.telegram.Verifier.Verify"

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		data *domain.ExtractedData
		err  error
	}

	var wg sync.WaitGroup
	results := make(chan result, len(keys))

	for _, key := range keys {
		wg.Add(1)
		go func(key domain.DatacenterKey) {
			defer wg.Done()

			data, err := v.tryKey(ctx, key)
			results <- result{data, err}
		}(key)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err == nil {
			v.log.Info("home key found", slog.Int("dc", r.data.DCID), slog.Int64("user_id", r.data.UserID))
			cancel()
			return r.data, nil
		}
	}

	return nil, fmt.Errorf("%s: %w: %d candidates", fn, domain.ErrVerificationFailed, len(keys))
}

func (v *Verifier) tryKey(ctx context.Context, key domain.DatacenterKey) (*domain.ExtractedData, error) {
	storage, err := v.buildSession(ctx, key)
	if err != nil {
		return nil, err
	}

	client := telegram.NewClient(v.cfg.ApiID, v.cfg.ApiHash, telegram.Options{
		SessionStorage: storage,
		DC:             key.DCID,
		DCList:         v.dcList,
	})

	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	var userID int64
	err = client.Run(attemptCtx, func(ctx context.Context) error {
		id, err := fetchSelfID(ctx, client.API())
		if err != nil {
			return err
		}

		userID = id
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &domain.ExtractedData{DCID: key.DCID, AuthKey: key.AuthKey, UserID: userID}, nil
}

func (v *Verifier) buildSession(ctx context.Context, key domain.DatacenterKey) (*session.StorageMemory, error) {
	addr, ok := v.dcAddr(key.DCID)
	if !ok {
		return nil, fmt.Errorf("no address for dc %d", key.DCID)
	}

	var k crypto.Key
	copy(k[:], key.AuthKey)
	authKey := k.WithID()

	storage := &session.StorageMemory{}
	loader := session.Loader{Storage: storage}

	if err := loader.Save(ctx, &session.Data{
		DC:        key.DCID,
		Addr:      addr,
		AuthKey:   authKey.Value[:],
		AuthKeyID: authKey.ID[:],
	}); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}

	return storage, nil
}

func fetchSelfID(ctx context.Context, api *tg.Client) (int64, error) {
	res, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
	if err != nil {
		return 0, err
	}

	if len(res) == 0 {
		return 0, fmt.Errorf("getUsers returned empty")
	}

	u, ok := res[0].(*tg.User)
	if !ok {
		return 0, fmt.Errorf("unexpected user type %T", res[0])
	}

	return u.ID, nil
}

func (v *Verifier) dcAddr(dc int) (string, bool) {
	for _, o := range v.dcList.Options {
		if o.ID == dc && !o.MediaOnly && !o.CDN && !o.Ipv6 {
			return net.JoinHostPort(o.IPAddress, strconv.Itoa(o.Port)), true
		}
	}

	return "", false
}
