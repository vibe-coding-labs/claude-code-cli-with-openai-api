package database

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	if err := InitEncryption(); err != nil {
		t.Fatalf("InitEncryption: %v", err)
	}

	plaintext := "sk-super-secret-api-key-1234567890"
	enc, err := EncryptAPIKey(plaintext)
	if err != nil {
		t.Fatalf("EncryptAPIKey: %v", err)
	}
	if enc == plaintext {
		t.Error("ciphertext should differ from plaintext")
	}

	dec, err := DecryptAPIKey(enc)
	if err != nil {
		t.Fatalf("DecryptAPIKey: %v", err)
	}
	if dec != plaintext {
		t.Errorf("round trip mismatch: got %q, want %q", dec, plaintext)
	}

	// Encrypting the same plaintext twice must yield different ciphertexts
	// because of the random nonce, but both must decrypt correctly.
	enc2, err := EncryptAPIKey(plaintext)
	if err != nil {
		t.Fatalf("EncryptAPIKey (2nd): %v", err)
	}
	if enc == enc2 {
		t.Error("expected distinct ciphertexts due to random nonce")
	}
	dec2, err := DecryptAPIKey(enc2)
	if err != nil || dec2 != plaintext {
		t.Errorf("second round trip failed: dec=%q err=%v", dec2, err)
	}
}

func TestEncryptAPIKey_KeyNotInitialized(t *testing.T) {
	prevKey := encryptionKey
	defer func() { encryptionKey = prevKey }()
	encryptionKey = nil

	if _, err := EncryptAPIKey("plaintext"); err == nil {
		t.Error("expected error when encryption key not initialized")
	}
	if _, err := DecryptAPIKey("anything"); err == nil {
		t.Error("expected error when encryption key not initialized")
	}
}

func TestDecryptAPIKey_InvalidInput(t *testing.T) {
	if err := InitEncryption(); err != nil {
		t.Fatalf("InitEncryption: %v", err)
	}

	// Not valid base64.
	if _, err := DecryptAPIKey("not-valid-base64!!!"); err == nil {
		t.Error("expected error for invalid base64 input")
	}

	// Valid base64 but too short to contain a nonce.
	if _, err := DecryptAPIKey("YQ=="); err == nil {
		t.Error("expected error for ciphertext shorter than nonce size")
	}

	// Valid base64, long enough, but tampered so GCM auth fails.
	enc, err := EncryptAPIKey("some secret value")
	if err != nil {
		t.Fatalf("EncryptAPIKey: %v", err)
	}
	tampered := strings.Replace(enc, enc[len(enc)-2:], "zz", 1)
	if tampered == enc {
		tampered = "AA" + enc[2:]
	}
	if _, err := DecryptAPIKey(tampered); err == nil {
		t.Error("expected GCM auth failure for tampered ciphertext")
	}
}

func TestMaskAPIKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "****"},
		{"short", "****"},
		{"exactly12chr", "****"}, // len == 12, boundary: <=12 -> masked
		{"thisislongerthan12", "thisislo...an12"},
	}
	for _, c := range cases {
		if got := MaskAPIKey(c.in); got != c.want {
			t.Errorf("MaskAPIKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
