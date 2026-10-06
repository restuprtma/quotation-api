package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const BlueprintShareTokenByteLength = 32

// GenerateShareToken generates a 32-byte cryptographically secure random token,
// encodes it to URL-safe base64 without padding, and calculates its lowercase SHA-256 hexadecimal hash.
func GenerateShareToken() (string, string, error) {
	bytes := make([]byte, BlueprintShareTokenByteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate secure random token: %w", err)
	}

	rawToken := base64.RawURLEncoding.EncodeToString(bytes)
	tokenHash := HashShareToken(rawToken)

	return rawToken, tokenHash, nil
}

// HashShareToken computes the lowercase hexadecimal SHA-256 hash of a raw token.
func HashShareToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}
