package utils

import (
	"encoding/base64"
	"testing"
)

func TestEncryptDecryptToken(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}

	if len(key) != 32 {
		t.Fatalf("Expected key length 32, got %d", len(key))
	}

	token := "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"

	// Test encryption
	encrypted, err := EncryptToken(token, key)
	if err != nil {
		t.Fatalf("Failed to encrypt token: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypted token is empty")
	}

	if encrypted == token {
		t.Fatal("Encrypted token should not equal original token")
	}

	// Test decryption
	decrypted, err := DecryptToken(encrypted, key)
	if err != nil {
		t.Fatalf("Failed to decrypt token: %v", err)
	}

	if decrypted != token {
		t.Fatalf("Decrypted token doesn't match original. Expected: %s, Got: %s", token, decrypted)
	}
}

func TestDecryptTokenWithWrongKey(t *testing.T) {
	key1, _ := GenerateEncryptionKey()
	key2, _ := GenerateEncryptionKey()

	token := "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
	encrypted, _ := EncryptToken(token, key1)

	// Try to decrypt with wrong key
	_, err := DecryptToken(encrypted, key2)
	if err == nil {
		t.Fatal("Decryption with wrong key should fail")
	}
}

func TestGetEncryptionKeyFromConfig(t *testing.T) {
	// Test with empty key (should generate new one)
	key1, err := GetEncryptionKeyFromConfig("")
	if err != nil {
		t.Fatalf("Failed to get encryption key from empty config: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("Expected key length 32, got %d", len(key1))
	}

	// Test with valid base64 key
	validKey := make([]byte, 32)
	for i := range validKey {
		validKey[i] = byte(i)
	}
	encodedKey := base64.StdEncoding.EncodeToString(validKey)

	key2, err := GetEncryptionKeyFromConfig(encodedKey)
	if err != nil {
		t.Fatalf("Failed to get encryption key from valid config: %v", err)
	}
	if len(key2) != 32 {
		t.Fatalf("Expected key length 32, got %d", len(key2))
	}
	for i := range key2 {
		if key2[i] != validKey[i] {
			t.Fatalf("Key mismatch at index %d", i)
		}
	}

	// Test with invalid base64
	_, err = GetEncryptionKeyFromConfig("invalid-base64!")
	if err == nil {
		t.Fatal("Should fail with invalid base64")
	}

	// Test with wrong length
	shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err = GetEncryptionKeyFromConfig(shortKey)
	if err == nil {
		t.Fatal("Should fail with wrong key length")
	}
}
