package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func encodeKey(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func decodeKey(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil, errors.New("not a 32-byte base64 WireGuard key")
	}
	return b, nil
}

// ValidPublicKey reports whether s has the shape of a WireGuard public key.
func ValidPublicKey(s string) bool {
	_, err := decodeKey(s)
	return err == nil
}

// GenerateKeyPair returns a WireGuard-compatible X25519 key pair.
func GenerateKeyPair() (priv, pub string, err error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return encodeKey(k.Bytes()), encodeKey(k.PublicKey().Bytes()), nil
}

// PublicFromPrivate derives the public key for a base64 private key.
func PublicFromPrivate(priv string) (string, error) {
	raw, err := decodeKey(priv)
	if err != nil {
		return "", err
	}
	k, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", err
	}
	return encodeKey(k.PublicKey().Bytes()), nil
}

// loadOrCreateKey returns the private key stored at path, creating it (0600)
// on first use.
func loadOrCreateKey(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		priv := strings.TrimSpace(string(b))
		if _, err := PublicFromPrivate(priv); err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		return priv, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	priv, _, err := GenerateKeyPair()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(priv+"\n"), 0o600); err != nil {
		return "", err
	}
	return priv, nil
}
