package service

import (
	"testing"
)

func TestCredentialCipher(t *testing.T) {
	key := "12345678901234567890123456789012" // 32 bytes
	cipher, err := NewCredentialCipher(key)
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	t.Run("Round-trip encrypt/decrypt", func(t *testing.T) {
		plaintext := "my-secret-api-key"
		ciphertext, err := cipher.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("failed to encrypt: %v", err)
		}

		decrypted, err := cipher.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("failed to decrypt: %v", err)
		}

		if decrypted != plaintext {
			t.Errorf("expected %q, got %q", plaintext, decrypted)
		}
	})

	t.Run("Random nonce", func(t *testing.T) {
		plaintext := "another-secret"
		c1, _ := cipher.Encrypt(plaintext)
		c2, _ := cipher.Encrypt(plaintext)

		if c1 == c2 {
			t.Error("expected different ciphertexts for the same plaintext due to random nonce")
		}
	})

	t.Run("Wrong-key detection", func(t *testing.T) {
		plaintext := "secret"
		ciphertext, _ := cipher.Encrypt(plaintext)

		wrongKey := "09876543210987654321098765432109" // 32 bytes
		wrongCipher, _ := NewCredentialCipher(wrongKey)

		_, err := wrongCipher.Decrypt(ciphertext)
		if err == nil {
			t.Error("expected error when decrypting with wrong key")
		}
		if err != nil && err.Error() != "decryption failed" {
			t.Errorf("expected safe error 'decryption failed', got %q", err.Error())
		}
	})

	t.Run("Tamper detection", func(t *testing.T) {
		plaintext := "secret"
		ciphertext, _ := cipher.Encrypt(plaintext)

		// Tamper with the base64 string
		tampered := ciphertext[:len(ciphertext)-5] + "A" + ciphertext[len(ciphertext)-4:]

		_, err := cipher.Decrypt(tampered)
		if err == nil {
			t.Error("expected error when decrypting tampered ciphertext")
		}
	})

	t.Run("Short ciphertext", func(t *testing.T) {
		// Just standard base64 but shorter than nonce
		short := "short"
		_, err := cipher.Decrypt(short)
		if err == nil {
			t.Error("expected error for too short ciphertext")
		}
	})
}
