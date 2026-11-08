package tgnet

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"fmt"

	"telegram-auth-extractor/internal/domain"
)

const (
	authKeySize    = 256
	datacenterVer  = 13
	tlBoolTrue     = 0x997275b5
	tlBoolFalse    = 0xbc799737
	maxDatacenters = 1024
)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(_ context.Context, data []byte) ([]domain.DatacenterKey, error) {
	const fn = "infra.tgnet.Parser.Parse"

	payload, err := unwrapFrame(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	cursor := &reader{buf: payload}

	cursor.u32()
	cursor.boolean()
	cursor.boolean()
	cursor.skipTLString()

	var keys []domain.DatacenterKey

	if cursor.boolean() {
		cursor.u32()
		cursor.i32()
		cursor.i32()
		cursor.i64()
		cursor.boolean()
		cursor.i32()

		sessions := cursor.u32()
		for range sessions {
			if cursor.err != nil {
				break
			}
			cursor.i64()
		}

		datacenterCount := cursor.u32()
		if datacenterCount > maxDatacenters {
			return nil, fmt.Errorf("%s: implausible datacenter count %d", fn, datacenterCount)
		}

		for range datacenterCount {
			if cursor.err != nil {
				break
			}

			datacenterID, authKey := cursor.datacenter()
			if cursor.err != nil {
				break
			}

			if len(authKey) != authKeySize {
				continue
			}

			sum := sha1.Sum(authKey)
			keys = append(keys, domain.DatacenterKey{
				DCID:    datacenterID,
				AuthKey: authKey,
				KeyID:   append([]byte(nil), sum[12:20]...),
			})
		}
	}

	if cursor.err != nil {
		return nil, fmt.Errorf("%s: %w", fn, cursor.err)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("%s: %w", fn, domain.ErrNoAuthKeys)
	}

	return keys, nil
}

func (r *reader) datacenter() (int, []byte) {
	version := r.u32()
	datacenterID := int(r.u32())
	if r.err != nil {
		return 0, nil
	}

	if version != datacenterVer {
		r.fail("unsupported datacenter version %d (expected %d)", version, datacenterVer)
		return 0, nil
	}

	r.u32()
	r.u32()

	for range 4 {
		addresses := r.u32()
		for range addresses {
			if r.err != nil {
				break
			}
			r.skipTLString()
			r.u32()
			r.i32()
			r.skipTLString()
		}
	}

	r.boolean()
	authKey := r.rawBytes()
	permID := r.i64()

	if len(authKey) == authKeySize {
		sum := sha1.Sum(authKey)
		if int64(binary.LittleEndian.Uint64(sum[12:20])) != permID {
			r.fail("dc %d: auth key id mismatch", datacenterID)
			return 0, nil
		}
	}

	r.skipRawBytes()
	r.i64()
	r.skipRawBytes()
	r.i64()
	r.i32()

	salts := r.u32()
	for range salts {
		if r.err != nil {
			break
		}
		r.i32()
		r.i32()
		r.i64()
	}

	mediaSalts := r.u32()
	for range mediaSalts {
		if r.err != nil {
			break
		}
		r.i32()
		r.i32()
		r.i64()
	}

	return datacenterID, authKey
}

func unwrapFrame(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("file too small (%d bytes)", len(data))
	}

	size := binary.LittleEndian.Uint32(data)
	body := data[4:]
	if size == 0 || int(size) > len(body) {
		return nil, fmt.Errorf("invalid frame size %d (payload %d bytes)", size, len(body))
	}

	return body[:size], nil
}
