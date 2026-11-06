package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"filippo.io/age/armor"
)

func main() {
	if len(os.Args) != 3 {
		_, _ = fmt.Fprintf(os.Stderr, "usage: %s <key-file> <auth-key-file>\n", os.Args[0])
		os.Exit(2)
	}

	if err := run(os.Args[1], os.Args[2]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(keyPath, authKeyPath string) error {
	keyFile, err := os.Open(keyPath)
	if err != nil {
		return fmt.Errorf("open key file: %w", err)
	}
	defer func() { _ = keyFile.Close() }()

	identities, err := age.ParseIdentities(keyFile)
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}

	authKeyFile, err := os.Open(authKeyPath)
	if err != nil {
		return fmt.Errorf("open auth key file: %w", err)
	}
	defer func() { _ = authKeyFile.Close() }()

	reader, err := age.Decrypt(armor.NewReader(authKeyFile), identities...)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	authKey, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read plaintext: %w", err)
	}

	fmt.Println(base64.StdEncoding.EncodeToString(authKey))
	return nil
}
