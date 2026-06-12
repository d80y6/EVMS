package common

import (
	"os"
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv(EncryptionKeyEnv)

	plaintext := "test-secret-value"
	encrypted, err := Encrypt([]byte(plaintext))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != plaintext {
		t.Errorf("round-trip failed: got %q, want %q", string(decrypted), plaintext)
	}
}

func TestEncryptNoKey(t *testing.T) {
	os.Unsetenv(EncryptionKeyEnv)

	_, err := Encrypt([]byte("test"))
	if err == nil {
		t.Error("expected error when ENCRYPTION_KEY is not set")
	}
}

func TestDecryptInvalid(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv(EncryptionKeyEnv)

	_, err := Decrypt("invalid-hex")
	if err == nil {
		t.Error("expected error for invalid ciphertext")
	}
}

func TestMustEncryptWithKey(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv(EncryptionKeyEnv)

	result := MustEncrypt("secret")
	if result == "secret" {
		t.Error("MustEncrypt returned plaintext when key is set")
	}
}

func TestMustEncryptWithoutKey(t *testing.T) {
	os.Unsetenv(EncryptionKeyEnv)

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustEncrypt should panic when key is missing")
		}
	}()
	MustEncrypt("secret")
}

func TestMustDecryptEmpty(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv(EncryptionKeyEnv)

	result := MustDecrypt("")
	if result != "" {
		t.Errorf("MustDecrypt('') = %q, want ''", result)
	}
}

func TestMustDecryptFailsGracefully(t *testing.T) {
	os.Unsetenv(EncryptionKeyEnv)

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustDecrypt should panic when key is missing")
		}
	}()
	MustDecrypt("some-encoded-data")
}

func TestMustEncryptDecryptRoundTrip(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv(EncryptionKeyEnv)

	original := "my-sso-client-secret"
	encrypted := MustEncrypt(original)
	if encrypted == original {
		t.Fatal("MustEncrypt returned plaintext")
	}

	decrypted := MustDecrypt(encrypted)
	if decrypted != original {
		t.Errorf("round-trip failed: got %q, want %q", decrypted, original)
	}
}

func TestMustEncryptPanicsWithoutKey(t *testing.T) {
	os.Unsetenv(EncryptionKeyEnv)

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustEncrypt should panic when key is missing")
		}
	}()
	MustEncrypt("test")
}

func TestGetEncryptionKey(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	defer os.Unsetenv(EncryptionKeyEnv)

	key, err := GetEncryptionKey()
	if err != nil {
		t.Fatalf("GetEncryptionKey failed: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(key))
	}
}

func TestGetEncryptionKeyMissing(t *testing.T) {
	os.Unsetenv(EncryptionKeyEnv)

	_, err := GetEncryptionKey()
	if err != ErrEncryptionKeyMissing {
		t.Errorf("expected ErrEncryptionKeyMissing, got %v", err)
	}
}

func TestGetEncryptionKeyInvalidFormat(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "not-a-valid-hex-string!")
	defer os.Unsetenv(EncryptionKeyEnv)

	_, err := GetEncryptionKey()
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected invalid format error, got %v", err)
	}
}

func TestGetEncryptionKeyWrongLength(t *testing.T) {
	os.Setenv(EncryptionKeyEnv, "aabb")
	defer os.Unsetenv(EncryptionKeyEnv)

	_, err := GetEncryptionKey()
	if err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("expected 32 bytes error, got %v", err)
	}
}
