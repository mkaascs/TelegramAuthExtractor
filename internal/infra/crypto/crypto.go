package crypto

import (
	"bytes"
	"fmt"

	"filippo.io/age"
	"filippo.io/age/armor"
)

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Generate() (recipient string, secret string, err error) {
	const fn = "infra.crypto.Manager.Generate"

	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", fn, err)
	}

	return id.Recipient().String(), id.String(), nil
}

func (m *Manager) Seal(recipient string, plaintext []byte) (string, error) {
	const fn = "infra.crypto.Manager.Seal"

	rcpt, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fn, err)
	}

	var buf bytes.Buffer
	armorWriter := armor.NewWriter(&buf)

	cryptWriter, err := age.Encrypt(armorWriter, rcpt)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fn, err)
	}

	if _, err := cryptWriter.Write(plaintext); err != nil {
		return "", fmt.Errorf("%s: %w", fn, err)
	}

	if err := cryptWriter.Close(); err != nil {
		return "", fmt.Errorf("%s: %w", fn, err)
	}

	if err := armorWriter.Close(); err != nil {
		return "", fmt.Errorf("%s: %w", fn, err)
	}

	return buf.String(), nil
}
