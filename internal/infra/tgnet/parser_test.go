package tgnet

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeKey(t *testing.T) (key []byte, id []byte) {
	t.Helper()
	key = make([]byte, authKeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)

	sum := sha1.Sum(key)
	id = append([]byte(nil), sum[12:20]...)
	return key, id
}

func header(version, dcID uint32) []byte {
	h := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(h[0:4], version)
	binary.LittleEndian.PutUint32(h[4:8], dcID)
	return h
}

func TestParser_FindsValidKey(t *testing.T) {
	key, id := makeKey(t)

	data := header(3708, 5)
	data = append(data, make([]byte, 32)...)
	data = append(data, key...)
	data = append(data, id...)
	data = append(data, make([]byte, 16)...)

	keys, err := NewParser().Parse(context.Background(), data)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, key, keys[0].AuthKey)
	require.Equal(t, id, keys[0].KeyID)
	require.Equal(t, 0, keys[0].DCID)
}

func TestParser_FindsMultipleKeys(t *testing.T) {
	k1, id1 := makeKey(t)
	k2, id2 := makeKey(t)
	k3, id3 := makeKey(t)

	data := header(3708, 2)
	for _, pair := range [][2][]byte{{k1, id1}, {k2, id2}, {k3, id3}} {
		data = append(data, make([]byte, 24)...)
		data = append(data, pair[0]...)
		data = append(data, pair[1]...)
	}

	keys, err := NewParser().Parse(context.Background(), data)
	require.NoError(t, err)
	require.Len(t, keys, 3)

	found := map[string]bool{}
	for _, key := range keys {
		found[string(key.KeyID)] = true
	}

	require.True(t, found[string(id1)])
	require.True(t, found[string(id2)])
	require.True(t, found[string(id3)])
}

func TestParser_Deduplicates(t *testing.T) {
	key, id := makeKey(t)

	data := header(3708, 5)
	data = append(data, key...)
	data = append(data, id...)
	data = append(data, key...)
	data = append(data, id...)

	keys, err := NewParser().Parse(context.Background(), data)
	require.NoError(t, err)
	require.Len(t, keys, 1, "duplicate by keyId must be eliminated")
}

func TestParser_IgnoresGarbage(t *testing.T) {
	data := make([]byte, 4096)
	_, err := rand.Read(data)
	require.NoError(t, err)

	_, parseErr := NewParser().Parse(context.Background(), data)
	require.Error(t, parseErr, "there should be no auth-keys in random garbage")
}

func TestParser_TamperedKeyRejected(t *testing.T) {
	key, id := makeKey(t)
	key[0] ^= 0xFF

	data := header(3708, 5)
	data = append(data, key...)
	data = append(data, id...)

	_, err := NewParser().Parse(context.Background(), data)
	require.Error(t, err, "key with a mismatched keyId should not be present")
}

func TestParser_TooSmall(t *testing.T) {
	_, err := NewParser().Parse(context.Background(), []byte{1, 2, 3})
	require.Error(t, err)
}
