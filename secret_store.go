package agent

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// ──────────────────────────────────────────────────────────────
// SecretStore – AES-256-GCM transparent encryption wrapper
// ──────────────────────────────────────────────────────────────

// SecretStore wraps a KVStore and transparently encrypts / decrypts values
// using AES-256-GCM. The master key is stored at masterKeyPath (chmod 0600).
type SecretStore struct {
	inner KVStore
	key   []byte // 32-byte AES-256 key
}

// NewSecretStore creates a SecretStore backed by inner.
// If masterKeyPath does not exist, a fresh 32-byte key is generated and saved.
func NewSecretStore(inner KVStore, masterKeyPath string) (*SecretStore, error) {
	key, err := loadOrGenerateKey(masterKeyPath)
	if err != nil {
		return nil, err
	}
	return &SecretStore{inner: inner, key: key}, nil
}

// EncryptValue encrypts plaintext with AES-256-GCM and returns a base64-encoded ciphertext.
func (s *SecretStore) EncryptValue(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptValue decrypts a base64-encoded AES-256-GCM ciphertext produced by EncryptValue.
func (s *SecretStore) DecryptValue(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func loadOrGenerateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) == 32 {
		return data, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate master key: %w", err)
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("failed to save master key to %s: %w", path, err)
	}
	return key, nil
}
