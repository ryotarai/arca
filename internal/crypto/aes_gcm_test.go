package crypto

import (
	"encoding/hex"
	"testing"
)

func testKey(seed string) string {
	// Pad or truncate seed to 32 bytes, then hex-encode.
	b := make([]byte, 32)
	copy(b, seed)
	return hex.EncodeToString(b)
}

func TestEncryptorRoundTrip(t *testing.T) {
	key := testKey("key1")

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
	key := testKey("key1")

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
	key := testKey("key1")

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

func TestEncryptorPrimaryOnlyBackwardCompatible(t *testing.T) {
	key := testKey("key1")

	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "test-backward-compat"
	encrypted, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWithOldKey(t *testing.T) {
	oldKey := testKey("old-key")
	newKey := testKey("new-key")

	// Encrypt with old key.
	oldEnc, err := NewEncryptor(oldKey)
	if err != nil {
		t.Fatalf("NewEncryptor old: %v", err)
	}
	encrypted, err := oldEnc.Encrypt("secret-data")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decrypt with new encryptor that has old key in fallback list.
	newEnc, err := NewEncryptor(newKey, oldKey)
	if err != nil {
		t.Fatalf("NewEncryptor new: %v", err)
	}

	decrypted, err := newEnc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt with old key fallback: %v", err)
	}
	if decrypted != "secret-data" {
		t.Fatalf("got %q, want %q", decrypted, "secret-data")
	}
}

func TestDecryptWithMultipleOldKeys(t *testing.T) {
	key1 := testKey("key-v1")
	key2 := testKey("key-v2")
	key3 := testKey("key-v3")

	// Encrypt with key1 (oldest).
	enc1, _ := NewEncryptor(key1)
	encrypted1, _ := enc1.Encrypt("data-from-v1")

	// Encrypt with key2.
	enc2, _ := NewEncryptor(key2)
	encrypted2, _ := enc2.Encrypt("data-from-v2")

	// New encryptor with key3 as primary, key1 and key2 as old.
	enc3, err := NewEncryptor(key3, key1, key2)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	// Should decrypt data from key1.
	d1, err := enc3.Decrypt(encrypted1)
	if err != nil {
		t.Fatalf("Decrypt key1 data: %v", err)
	}
	if d1 != "data-from-v1" {
		t.Fatalf("got %q, want %q", d1, "data-from-v1")
	}

	// Should decrypt data from key2.
	d2, err := enc3.Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("Decrypt key2 data: %v", err)
	}
	if d2 != "data-from-v2" {
		t.Fatalf("got %q, want %q", d2, "data-from-v2")
	}

	// Should also decrypt data encrypted with primary key3.
	encrypted3, _ := enc3.Encrypt("data-from-v3")
	d3, err := enc3.Decrypt(encrypted3)
	if err != nil {
		t.Fatalf("Decrypt key3 data: %v", err)
	}
	if d3 != "data-from-v3" {
		t.Fatalf("got %q, want %q", d3, "data-from-v3")
	}
}

func TestDecryptFailsWithUnknownKey(t *testing.T) {
	key1 := testKey("key-known")
	key2 := testKey("key-unknown")

	enc1, _ := NewEncryptor(key1)
	encrypted, _ := enc1.Encrypt("secret")

	// Encryptor with a completely different key.
	enc2, _ := NewEncryptor(key2)
	_, err := enc2.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error decrypting with unknown key")
	}
}

func TestInvalidOldKey(t *testing.T) {
	primaryKey := testKey("primary")

	_, err := NewEncryptor(primaryKey, "not-hex")
	if err == nil {
		t.Fatal("expected error for invalid old key")
	}

	_, err = NewEncryptor(primaryKey, "")
	if err == nil {
		t.Fatal("expected error for empty old key")
	}
}

func TestReEncryptFromOldKey(t *testing.T) {
	oldKey := testKey("old-key")
	newKey := testKey("new-key")

	// Encrypt with old key.
	oldEnc, _ := NewEncryptor(oldKey)
	encrypted, _ := oldEnc.Encrypt("api-key-value")

	// Re-encrypt with new encryptor.
	newEnc, _ := NewEncryptor(newKey, oldKey)
	reEncrypted, changed, err := newEnc.ReEncrypt(encrypted)
	if err != nil {
		t.Fatalf("ReEncrypt: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for old-key ciphertext")
	}
	if reEncrypted == encrypted {
		t.Fatal("re-encrypted ciphertext should differ from original")
	}

	// Verify re-encrypted data can be decrypted with primary key only.
	primaryOnlyEnc, _ := NewEncryptor(newKey)
	decrypted, err := primaryOnlyEnc.Decrypt(reEncrypted)
	if err != nil {
		t.Fatalf("Decrypt re-encrypted: %v", err)
	}
	if decrypted != "api-key-value" {
		t.Fatalf("got %q, want %q", decrypted, "api-key-value")
	}
}

func TestReEncryptAlreadyPrimaryKey(t *testing.T) {
	key := testKey("primary")

	enc, _ := NewEncryptor(key)
	encrypted, _ := enc.Encrypt("already-primary")

	reEncrypted, changed, err := enc.ReEncrypt(encrypted)
	if err != nil {
		t.Fatalf("ReEncrypt: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false for primary-key ciphertext")
	}
	if reEncrypted != encrypted {
		t.Fatal("expected original ciphertext returned unchanged")
	}
}

func TestReEncryptFailsWithUnknownKey(t *testing.T) {
	key1 := testKey("key-a")
	key2 := testKey("key-b")
	key3 := testKey("key-c")

	enc1, _ := NewEncryptor(key1)
	encrypted, _ := enc1.Encrypt("secret")

	// Encryptor that doesn't know about key1.
	enc2, _ := NewEncryptor(key2, key3)
	_, _, err := enc2.ReEncrypt(encrypted)
	if err == nil {
		t.Fatal("expected error when no key can decrypt")
	}
}
