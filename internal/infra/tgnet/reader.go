package tgnet

import (
	"encoding/binary"
	"fmt"
)

type reader struct {
	buf []byte
	pos int
	err error
}

func (r *reader) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf(format, args...)
	}
}

func (r *reader) need(size int) bool {
	if r.err != nil {
		return false
	}

	if size < 0 || r.pos+size > len(r.buf) {
		r.fail("unexpected end of data: need %d bytes at offset %d, have %d", size, r.pos, len(r.buf)-r.pos)
		return false
	}

	return true
}

func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}

	value := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return value
}

func (r *reader) i32() int32 {
	return int32(r.u32())
}

func (r *reader) i64() int64 {
	if !r.need(8) {
		return 0
	}

	value := binary.LittleEndian.Uint64(r.buf[r.pos:])
	r.pos += 8
	return int64(value)
}

func (r *reader) boolean() bool {
	switch r.u32() {
	case tlBoolTrue:
		return true
	case tlBoolFalse:
		return false
	default:
		r.fail("invalid bool tag at offset %d", r.pos-4)
		return false
	}
}

func (r *reader) rawBytes() []byte {
	length := int(r.u32())
	if r.err != nil || !r.need(length) {
		return nil
	}

	value := append([]byte(nil), r.buf[r.pos:r.pos+length]...)
	r.pos += length
	return value
}

func (r *reader) skipRawBytes() {
	length := int(r.u32())
	if r.err != nil || !r.need(length) {
		return
	}

	r.pos += length
}

func (r *reader) skipTLString() {
	if !r.need(1) {
		return
	}

	length := int(r.buf[r.pos])
	r.pos++
	header := 1

	if length >= 254 {
		if !r.need(3) {
			return
		}
		length = int(r.buf[r.pos]) | int(r.buf[r.pos+1])<<8 | int(r.buf[r.pos+2])<<16
		r.pos += 3
		header = 4
	}

	padding := 0
	if remainder := (length + header) % 4; remainder != 0 {
		padding = 4 - remainder
	}

	if !r.need(length + padding) {
		return
	}

	r.pos += length + padding
}
