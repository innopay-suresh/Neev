package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	// Argon2id parameters — tuned for interactive login (< 500ms on modern CPU).
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword derives an Argon2id hash of the given password.
// Returns a base64-encoded "salt:hash" string.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	encoded := base64.StdEncoding.EncodeToString(salt) + ":" + base64.StdEncoding.EncodeToString(hash)
	return encoded, nil
}

// VerifyPassword checks whether password matches the stored hash string.
func VerifyPassword(password, stored string) (bool, error) {
	// Parse "salt:hash"
	saltB64, hashB64, found := splitOnce(stored, ':')
	if !found {
		return false, fmt.Errorf("invalid hash format")
	}
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	expectedHash, err := base64.StdEncoding.DecodeString(hashB64)
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}
	actualHash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return constantTimeEqual(actualHash, expectedHash), nil
}

// splitOnce splits s on the first occurrence of sep.
func splitOnce(s string, sep byte) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// constantTimeEqual compares two byte slices in constant time.
func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// GenerateRandomPassword generates a cryptographically random 8-character
// alphanumeric password suitable for one-time session passwords.
func GenerateRandomPassword() (string, error) {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}
