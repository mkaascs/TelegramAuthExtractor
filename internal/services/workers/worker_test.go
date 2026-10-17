package workers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
)

type mockRepo struct {
	mu       sync.Mutex
	session  *domain.Session
	getErr   error
	statuses []domain.Status
}

func (m *mockRepo) Get(_ context.Context, _ uuid.UUID) (*domain.Session, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.session, nil
}

func (m *mockRepo) Update(_ context.Context, s *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = append(m.statuses, s.Status)
	return nil
}

func (m *mockRepo) snapshot() []domain.Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Status, len(m.statuses))
	copy(out, m.statuses)
	return out
}

type mockDevice struct {
	mu    sync.Mutex
	calls []string

	prepareErr error
	phoneErr   error
	codeErr    error
	file       []byte
	fileErr    error
	clearErr   error
}

func (d *mockDevice) record(c string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, c)
}

func (d *mockDevice) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.calls))
	copy(out, d.calls)
	return out
}

func (d *mockDevice) PrepareTelegram(_ context.Context) error {
	d.record("prepare")
	return d.prepareErr
}

func (d *mockDevice) EnterPhone(_ context.Context, _ string) error {
	d.record("phone")
	return d.phoneErr
}

func (d *mockDevice) EnterCode(_ context.Context, _ string) error {
	d.record("code")
	return d.codeErr
}

func (d *mockDevice) SessionFile(_ context.Context) ([]byte, error) {
	d.record("file")
	return d.file, d.fileErr
}

func (d *mockDevice) Clear(_ context.Context) error {
	d.record("clear")
	return d.clearErr
}

type mockParser struct {
	keys []domain.DatacenterKey
	err  error
}

func (p mockParser) Parse(_ context.Context, _ []byte) ([]domain.DatacenterKey, error) {
	return p.keys, p.err
}

type mockVerifier struct {
	result *domain.ExtractedData
	err    error
}

func (v mockVerifier) Verify(_ context.Context, _ []domain.DatacenterKey) (*domain.ExtractedData, error) {
	return v.result, v.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testCfg() config.Worker {
	return config.Worker{
		QueueSize:      8,
		CodeWait:       2 * time.Second,
		CleanupTimeout: time.Second,
	}
}

func deliverCode(t *testing.T, w *Worker, id uuid.UUID, code string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("worker never accepted the code")
		case <-tick.C:
			if err := w.SubmitCode(id, code); err == nil {
				return
			}
		}
	}
}

func cancelWhenReady(t *testing.T, w *Worker, id uuid.UUID) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("worker never became cancellable")
		case <-tick.C:
			if err := w.Cancel(id); err == nil {
				return
			}
		}
	}
}

func TestWorker_Process_HappyPath(t *testing.T) {
	session := domain.NewSession("+79991234567")
	id := uuid.UUID(session.ID)
	result := &domain.ExtractedData{DCID: 2, AuthKey: []byte("key"), UserID: 1153367122}

	repo := &mockRepo{session: session}
	device := &mockDevice{file: []byte("tgnet")}
	w := NewWorker(repo, device,
		mockParser{keys: []domain.DatacenterKey{{DCID: 2}}},
		mockVerifier{result: result},
		discardLogger(), testCfg())

	done := make(chan struct{})
	go func() { w.process(context.Background(), session); close(done) }()

	deliverCode(t, w, id, "12345")
	<-done

	require.Equal(t, []domain.Status{
		domain.StatusLoggingIn,
		domain.StatusWaitingForCode,
		domain.StatusEnteringCode,
		domain.StatusExtracting,
		domain.StatusCleanup,
		domain.StatusDone,
	}, repo.snapshot())

	require.Equal(t, []string{"prepare", "phone", "code", "file", "clear"}, device.snapshot())
	require.Equal(t, domain.StatusDone, session.Status)
	require.Equal(t, result, session.Result)
	require.Nil(t, session.Error)
}

func TestWorker_Process_CodeTimeout(t *testing.T) {
	session := domain.NewSession("+79991234567")

	cfg := testCfg()
	cfg.CodeWait = 50 * time.Millisecond

	repo := &mockRepo{session: session}
	device := &mockDevice{}
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, discardLogger(), cfg)

	w.process(context.Background(), session)

	require.Equal(t, domain.StatusFailed, session.Status)
	require.NotNil(t, session.Error)

	require.Equal(t, []string{"prepare", "phone", "clear"}, device.snapshot())
	require.Equal(t, []domain.Status{
		domain.StatusLoggingIn,
		domain.StatusWaitingForCode,
		domain.StatusCleanup,
		domain.StatusFailed,
	}, repo.snapshot())
}

func TestWorker_Process_Cancelled(t *testing.T) {
	session := domain.NewSession("+79991234567")
	id := uuid.UUID(session.ID)

	repo := &mockRepo{session: session}
	device := &mockDevice{}
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, discardLogger(), testCfg())

	done := make(chan struct{})
	go func() { w.process(context.Background(), session); close(done) }()

	cancelWhenReady(t, w, id)
	<-done

	require.NotEqual(t, domain.StatusFailed, session.Status)
	require.NotEqual(t, domain.StatusDone, session.Status)
	require.Nil(t, session.Error)
	require.Contains(t, device.snapshot(), "clear")
}

func TestWorker_Process_DeviceError(t *testing.T) {
	session := domain.NewSession("+79991234567")

	repo := &mockRepo{session: session}
	device := &mockDevice{phoneErr: errors.New("adb: no device")}
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, discardLogger(), testCfg())

	w.process(context.Background(), session)

	require.Equal(t, domain.StatusFailed, session.Status)
	require.NotNil(t, session.Error)
	require.Equal(t, []string{"prepare", "phone", "clear"}, device.snapshot())
}

func TestWorker_Process_ParserError(t *testing.T) {
	session := domain.NewSession("+79991234567")
	id := uuid.UUID(session.ID)

	repo := &mockRepo{session: session}
	device := &mockDevice{file: []byte("tgnet")}
	w := NewWorker(repo, device,
		mockParser{err: errors.New("bad tgnet format")},
		mockVerifier{},
		discardLogger(), testCfg())

	done := make(chan struct{})
	go func() { w.process(context.Background(), session); close(done) }()

	deliverCode(t, w, id, "12345")
	<-done

	require.Equal(t, domain.StatusFailed, session.Status)
	require.NotNil(t, session.Error)
	require.Nil(t, session.Result)
	require.Contains(t, device.snapshot(), "clear") // cleanup всё равно
}

func TestWorker_Process_DeletedWhileQueued(t *testing.T) {
	session := domain.NewSession("+79991234567")

	repo := &mockRepo{getErr: domain.ErrSessionNotFound}
	device := &mockDevice{}
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, discardLogger(), testCfg())

	w.process(context.Background(), session)

	require.Empty(t, device.snapshot(), "по удалённой задаче устройство не трогаем")
	require.Empty(t, repo.snapshot(), "и статусы не меняем")
}

func TestWorker_SubmitCode_Idle(t *testing.T) {
	w := NewWorker(&mockRepo{}, &mockDevice{}, mockParser{}, mockVerifier{}, discardLogger(), testCfg())
	err := w.SubmitCode(uuid.New(), "12345")
	require.ErrorIs(t, err, domain.ErrWrongState)
}

func TestWorker_Cancel_Idle(t *testing.T) {
	w := NewWorker(&mockRepo{}, &mockDevice{}, mockParser{}, mockVerifier{}, discardLogger(), testCfg())
	err := w.Cancel(uuid.New())
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}
