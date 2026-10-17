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

	for index := 0; index+authKeySize+keyIDSize <= len(data); index++ {
		key := data[index : index+authKeySize]
		id := data[index+authKeySize : index+authKeySize+keyIDSize]

		sum := sha1.Sum(key)
		if !bytes.Equal(sum[12:20], id) {
			continue
		}

		fp := string(id)
		if _, ok := seen[fp]; ok {
			continue
		}

		seen[fp] = struct{}{}

		keys = append(keys, domain.DatacenterKey{
			DCID:    0,
			AuthKey: append([]byte(nil), key...),
			KeyID:   append([]byte(nil), id...),
		})
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("%s: no auth keys found (not logged in?)", fn)
	}

	return keys, nil
}
