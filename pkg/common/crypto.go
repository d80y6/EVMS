package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

const EncryptionKeyEnv = "ENCRYPTION_KEY"

var (
	ErrEncryptionKeyMissing = errors.New("encryption key not set")
	ErrInvalidCiphertext    = errors.New("invalid ciphertext")
)

func GetEncryptionKey() ([]byte, error) {
	keyHex := os.Getenv(EncryptionKeyEnv)
	if keyHex == "" {
		return nil, ErrEncryptionKeyMissing
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, errors.New("invalid encryption key format")
	}
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}
	return key, nil
}

func Encrypt(plaintext []byte) (string, error) {
	key, err := GetEncryptionKey()
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
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(ciphertext), nil
}

func Decrypt(encoded string) ([]byte, error) {
	key, err := GetEncryptionKey()
	if err != nil {
		return nil, err
	}
	data, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, ErrInvalidCiphertext
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

func MustEncrypt(plaintext string) string {
	encrypted, err := Encrypt([]byte(plaintext))
	if err != nil {
		panic("MustEncrypt failed: " + err.Error())
	}
	return encrypted
}

func MustDecrypt(encoded string) string {
	if encoded == "" {
		return ""
	}
	decrypted, err := Decrypt(encoded)
	if err != nil {
		panic("MustDecrypt failed: " + err.Error())
	}
	return string(decrypted)
}
