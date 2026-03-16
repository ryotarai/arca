package crypto

import (
	"encoding/hex"
	"testing"
)

func TestEncryptorRoundTrip(t *testing.T) {
	// 32-byte key in hex
	key := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "sk-my-secret-api-key-12345"
	encrypted, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("encrypted should differ from plaintext")
	}

	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptorEmptyPlaintext(t *testing.T) {
	key := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	encrypted, err := enc.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if decrypted != "" {
		t.Fatalf("decrypted = %q, want empty string", decrypted)
	}
}

func TestEncryptorInvalidKey(t *testing.T) {
	if _, err := NewEncryptor(""); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, err := NewEncryptor("not-hex"); err == nil {
		t.Fatal("expected error for non-hex key")
	}
	// 15 bytes (too short for AES)
	if _, err := NewEncryptor(hex.EncodeToString([]byte("short-key-12345"))); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestEncryptorDifferentCiphertexts(t *testing.T) {
	key := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "same-secret"
	e1, _ := enc.Encrypt(plaintext)
	e2, _ := enc.Encrypt(plaintext)

	if e1 == e2 {
		t.Fatal("two encryptions of same plaintext should produce different ciphertexts (random nonce)")
	}
}
