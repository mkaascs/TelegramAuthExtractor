package tgnet

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"telegram-auth-extractor/internal/domain"
)

type writer struct{ buf []byte }

func (w *writer) u32(value uint32) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], value)
	w.buf = append(w.buf, tmp[:]...)
}

func (w *writer) i32(value int32) { w.u32(uint32(value)) }

func (w *writer) i64(value int64) {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], uint64(value))
	w.buf = append(w.buf, tmp[:]...)
}

func (w *writer) boolean(value bool) {
	if value {
		w.u32(tlBoolTrue)
	} else {
		w.u32(tlBoolFalse)
	}
}

func (w *writer) rawBytes(data []byte) {
	w.u32(uint32(len(data)))
	w.buf = append(w.buf, data...)
}

func (w *writer) tlString(data []byte) {
	length, header := len(data), 1
	if length <= 253 {
		w.buf = append(w.buf, byte(length))
	} else {
		w.buf = append(w.buf, 0xFE, byte(length), byte(length>>8), byte(length>>16))
		header = 4
	}
	w.buf = append(w.buf, data...)
	if remainder := (length + header) % 4; remainder != 0 {
		w.buf = append(w.buf, make([]byte, 4-remainder)...)
	}
}

func dcBlock(id int, authKey []byte, permID int64, addrs []string) []byte {
	w := &writer{}
	w.u32(13)
	w.u32(uint32(id))
	w.u32(0)
	w.u32(0)

	for list := range 4 {
		if list != 0 {
			w.u32(0)
			continue
		}
		w.u32(uint32(len(addrs)))
		for _, address := range addrs {
			w.tlString([]byte(address))
			w.u32(443)
			w.i32(0)
			w.tlString(nil)
		}
	}

	w.boolean(false)
	w.rawBytes(authKey)
	w.i64(permID)
	w.rawBytes(nil)
	w.i64(0)
	w.rawBytes(nil)
	w.i64(0)
	w.i32(1)
	w.u32(0)
	w.u32(0)
	return w.buf
}

func configBlob(homeID int, hasCurrent bool, datacenters ...[]byte) []byte {
	w := &writer{}
	w.u32(5)
	w.boolean(false)
	w.boolean(false)
	w.tlString([]byte("en"))
	w.boolean(hasCurrent)
	if hasCurrent {
		w.u32(uint32(homeID))
		w.i32(0)
		w.i32(0)
		w.i64(0)
		w.boolean(false)
		w.i32(0)
		w.u32(0)
		w.u32(uint32(len(datacenters)))
		for _, block := range datacenters {
			w.buf = append(w.buf, block...)
		}
	}
	return w.buf
}

func frame(payload []byte) []byte {
	w := &writer{}
	w.u32(uint32(len(payload)))
	w.buf = append(w.buf, payload...)
	return w.buf
}

func makeKey(t *testing.T) (key []byte, permID int64) {
	t.Helper()
	key = make([]byte, authKeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)

	sum := sha1.Sum(key)
	permID = int64(binary.LittleEndian.Uint64(sum[12:20]))
	return key, permID
}

func keyByDC(keys []domain.DatacenterKey) map[int][]byte {
	result := make(map[int][]byte, len(keys))
	for _, key := range keys {
		result[key.DCID] = key.AuthKey
	}
	return result
}

func TestParser_ExtractsDCAndKey(t *testing.T) {
	kA, idA := makeKey(t)
	blob := frame(configBlob(2, true, dcBlock(2, kA, idA, nil)))

	keys, err := NewParser().Parse(context.Background(), blob)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, 2, keys[0].DCID)
	require.Equal(t, kA, keys[0].AuthKey)

	sum := sha1.Sum(kA)
	require.Equal(t, sum[12:20], keys[0].KeyID)
}

func TestParser_MultipleDatacenters(t *testing.T) {
	k1, id1 := makeKey(t)
	k2, id2 := makeKey(t)
	k4, id4 := makeKey(t)

	blob := frame(configBlob(2, true,
		dcBlock(1, k1, id1, nil),
		dcBlock(2, k2, id2, nil),
		dcBlock(4, k4, id4, nil),
	))

	keys, err := NewParser().Parse(context.Background(), blob)
	require.NoError(t, err)
	require.Len(t, keys, 3)

	byDC := keyByDC(keys)
	require.Equal(t, k1, byDC[1])
	require.Equal(t, k2, byDC[2])
	require.Equal(t, k4, byDC[4])
}

func TestParser_SkipsKeylessDatacenter(t *testing.T) {
	kB, idB := makeKey(t)

	blob := frame(configBlob(2, true,
		dcBlock(1, nil, 0, nil),
		dcBlock(2, kB, idB, nil),
	))

	keys, err := NewParser().Parse(context.Background(), blob)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, 2, keys[0].DCID)
}

func TestParser_ParsesDatacenterWithAddresses(t *testing.T) {
	kB, idB := makeKey(t)

	blob := frame(configBlob(2, true,
		dcBlock(2, kB, idB, []string{"149.154.167.51", "2001:67c:4e8:f002::a"}),
	))

	keys, err := NewParser().Parse(context.Background(), blob)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, kB, keys[0].AuthKey)
}

func TestParser_NoCurrentDatacenter(t *testing.T) {
	blob := frame(configBlob(0, false))

	_, err := NewParser().Parse(context.Background(), blob)
	require.ErrorIs(t, err, domain.ErrNoAuthKeys)
}

func TestParser_AllKeyless(t *testing.T) {
	blob := frame(configBlob(2, true, dcBlock(2, nil, 0, nil)))

	_, err := NewParser().Parse(context.Background(), blob)
	require.ErrorIs(t, err, domain.ErrNoAuthKeys)
}

func TestParser_DesyncDetected(t *testing.T) {
	kB, idB := makeKey(t)

	blob := frame(configBlob(2, true, dcBlock(2, kB, idB^1, nil)))

	_, err := NewParser().Parse(context.Background(), blob)
	require.Error(t, err)
	require.NotErrorIs(t, err, domain.ErrNoAuthKeys)
}

func TestParser_UnsupportedVersion(t *testing.T) {
	kB, idB := makeKey(t)
	dc := dcBlock(2, kB, idB, nil)
	binary.LittleEndian.PutUint32(dc[0:4], 99)

	blob := frame(configBlob(2, true, dc))

	_, err := NewParser().Parse(context.Background(), blob)
	require.Error(t, err)
	require.NotErrorIs(t, err, domain.ErrNoAuthKeys)
}

func TestParser_Garbage(t *testing.T) {
	payload := make([]byte, 4096)
	_, err := rand.Read(payload)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		_, parseErr := NewParser().Parse(context.Background(), frame(payload))
		require.Error(t, parseErr)
	})
}

func TestParser_TruncatedDatacenter(t *testing.T) {
	kB, idB := makeKey(t)
	payload := configBlob(2, true, dcBlock(2, kB, idB, nil))
	payload = payload[:len(payload)-10]

	require.NotPanics(t, func() {
		_, err := NewParser().Parse(context.Background(), frame(payload))
		require.Error(t, err)
	})
}

func TestParser_BadFrameSize(t *testing.T) {
	payload := configBlob(2, true, dcBlock(2, []byte{1, 2, 3}, 0, nil))
	blob := frame(payload)
	binary.LittleEndian.PutUint32(blob[0:4], uint32(len(payload)+100))

	_, err := NewParser().Parse(context.Background(), blob)
	require.Error(t, err)
}

func TestParser_TooSmall(t *testing.T) {
	_, err := NewParser().Parse(context.Background(), []byte{1, 2, 3})
	require.Error(t, err)
}
