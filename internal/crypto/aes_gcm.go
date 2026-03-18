package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Encryptor provides AES-GCM encryption/decryption using a primary key and
// optional old keys for decryption during key rotation.
type Encryptor struct {
	primaryGCM  cipher.AEAD
	decryptGCMs []cipher.AEAD // old keys, used for decryption only
}

// NewEncryptor creates an Encryptor from a hex-encoded primary AES key and
// optional old hex-encoded keys used only for decryption.
func NewEncryptor(primaryHexKey string, oldHexKeys ...string) (*Encryptor, error) {
	primaryGCM, err := newGCM(primaryHexKey)
	if err != nil {
		return nil, fmt.Errorf("primary key: %w", err)
	}

	decryptGCMs := make([]cipher.AEAD, 0, len(oldHexKeys))
	for i, oldKey := range oldHexKeys {
		gcm, err := newGCM(oldKey)
		if err != nil {
			return nil, fmt.Errorf("old key[%d]: %w", i, err)
		}
		decryptGCMs = append(decryptGCMs, gcm)
	}

	return &Encryptor{
		primaryGCM:  primaryGCM,
		decryptGCMs: decryptGCMs,
	}, nil
}

func newGCM(hexKey string) (cipher.AEAD, error) {
	hexKey = strings.TrimSpace(hexKey)
	if hexKey == "" {
		return nil, errors.New("encryption key must not be empty")
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("invalid AES key length: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return gcm, nil
}

// Encrypt encrypts plaintext using the primary key and returns a base64-encoded ciphertext string.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.primaryGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := e.primaryGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext string. It tries the primary key
// first, then falls back to old keys in order.
func (e *Encryptor) Decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	// Try primary key first.
	if plaintext, err := decryptWithGCM(e.primaryGCM, data); err == nil {
		return plaintext, nil
	}

	// Try old keys.
	for _, gcm := range e.decryptGCMs {
		if plaintext, err := decryptWithGCM(gcm, data); err == nil {
			return plaintext, nil
		}
	}

	return "", errors.New("decrypt: failed with all keys")
}

// ReEncrypt decrypts ciphertext and re-encrypts it with the primary key.
// If the ciphertext is already encrypted with the primary key, it returns
// the original ciphertext with changed=false.
func (e *Encryptor) ReEncrypt(ciphertext string) (newCiphertext string, changed bool, err error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", false, fmt.Errorf("decode base64: %w", err)
	}

	// If primary key can decrypt it, no re-encryption needed.
	if _, err := decryptWithGCM(e.primaryGCM, data); err == nil {
		return ciphertext, false, nil
	}

	// Try old keys to decrypt.
	var plaintext string
	var decrypted bool
	for _, gcm := range e.decryptGCMs {
		if pt, err := decryptWithGCM(gcm, data); err == nil {
			plaintext = pt
			decrypted = true
			break
		}
	}

	if !decrypted {
		return "", false, errors.New("re-encrypt: failed to decrypt with any key")
	}

	// Re-encrypt with primary key.
	newCiphertext, err = e.Encrypt(plaintext)
	if err != nil {
		return "", false, fmt.Errorf("re-encrypt: %w", err)
	}

	return newCiphertext, true, nil
}

func decryptWithGCM(gcm cipher.AEAD, data []byte) (string, error) {
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
