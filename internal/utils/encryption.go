package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

func EncryptToken(token string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(token), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptToken(encryptedToken string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

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

func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// GetEncryptionKeyFromConfig decodes base64 encoded encryption key from config
// environment parameter determines if we should allow random key generation
// In production environment, encryption_key must be explicitly configured
func GetEncryptionKeyFromConfig(encodedKey string, environment string) ([]byte, error) {
	if encodedKey == "" {
		// In production environment, encryption_key is required to prevent data loss
		if environment == "production" {
			return nil, errors.New("CRITICAL SECURITY ERROR: encryption_key is missing in production environment. System cannot start to prevent data loss")
		}

		// Development environment allows automatic key generation for convenience
		return GenerateEncryptionKey()
	}

	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, errors.New("invalid encryption key format (must be base64)")
	}

	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	return key, nil
}
