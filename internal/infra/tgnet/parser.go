package tgnet

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"fmt"

	"telegram-auth-extractor/internal/domain"
)

const (
	authKeySize = 256
	keyIDSize   = 8
	headerSize  = 8
	clusterStep = 0x10C
)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(_ context.Context, data []byte) ([]domain.DatacenterKey, error) {
	const fn = "adapters.tgnet.Parser.Parse"

	if len(data) < headerSize {
		return nil, fmt.Errorf("%s: file too small (%d bytes)", fn, len(data))
	}

	version := binary.LittleEndian.Uint32(data[0:4])
	_ = version

	var keys []domain.DatacenterKey
	seen := make(map[string]struct{})
	lastOffset := -1

	for i := 0; i+authKeySize+keyIDSize <= len(data); i++ {
		key := data[i : i+authKeySize]
		id := data[i+authKeySize : i+authKeySize+keyIDSize]

		sum := sha1.Sum(key)
		if !bytes.Equal(sum[12:20], id) {
			continue
		}

		if _, ok := seen[string(id)]; ok {
			continue
		}

		seen[string(id)] = struct{}{}

		if lastOffset >= 0 && i-lastOffset == clusterStep {
			lastOffset = i
			continue
		}

		lastOffset = i

		keys = append(keys, domain.DatacenterKey{
			DCID:    0,
			AuthKey: append([]byte(nil), key...),
			KeyID:   append([]byte(nil), id...),
		})
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("%s: %w", fn, domain.ErrNoAuthKeys)
	}

	return keys, nil
}
