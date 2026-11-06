package crypto

import (
	"bytes"
	"io"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/stretchr/testify/require"
)

func decryptWith(t *testing.T, secret, ciphertext string) ([]byte, error) {
	t.Helper()

	id, err := age.ParseX25519Identity(secret)
	require.NoError(t, err)

	reader, err := age.Decrypt(armor.NewReader(bytes.NewReader([]byte(ciphertext))), id)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(reader)
}

func TestManager_SealDecrypt_RoundTrip(t *testing.T) {
	m := NewManager()

	recipient, secret, err := m.Generate()
	require.NoError(t, err)
	require.NotEmpty(t, recipient)
	require.NotEmpty(t, secret)

	authKey := bytes.Repeat([]byte{0xAB}, 256)

	ciphertext, err := m.Seal(recipient, authKey)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)
	require.NotContains(t, ciphertext, string(authKey), "плейнтекст не должен присутствовать в шифротексте")

	got, err := decryptWith(t, secret, ciphertext)
	require.NoError(t, err)
	require.Equal(t, authKey, got)
}

func TestManager_Seal_WrongSecretCannotOpen(t *testing.T) {
	m := NewManager()

	recipient, _, err := m.Generate()
	require.NoError(t, err)

	_, otherSecret, err := m.Generate()
	require.NoError(t, err)

	ciphertext, err := m.Seal(recipient, []byte("secret-data"))
	require.NoError(t, err)

	_, err = decryptWith(t, otherSecret, ciphertext)
	require.Error(t, err, "чужой приватный ключ не должен расшифровывать")
}

func TestManager_Seal_InvalidRecipient(t *testing.T) {
	m := NewManager()

	_, err := m.Seal("not-a-valid-recipient", []byte("x"))
	require.Error(t, err)
}

func TestManager_Generate_ProducesUniqueKeys(t *testing.T) {
	m := NewManager()

	r1, s1, err := m.Generate()
	require.NoError(t, err)

	r2, s2, err := m.Generate()
	require.NoError(t, err)

	require.NotEqual(t, r1, r2)
	require.NotEqual(t, s1, s2)
}
