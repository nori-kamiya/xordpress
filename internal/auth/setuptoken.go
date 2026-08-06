package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// setupTokenBytes is the number of random bytes read from crypto/rand when
// generating a setup token. 32 bytes gives 256 bits of entropy.
const setupTokenBytes = 32

// GenerateSetupToken generates a cryptographically random setup token,
// encoded as base64. It reads at least 32 bytes from crypto/rand.
func GenerateSetupToken() (string, error) {
	buf := make([]byte, setupTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf), nil
}

// CompareSetupToken reports whether provided matches expected, using a
// constant-time comparison to avoid leaking timing information.
//
// If expected is empty (e.g. because setup has already completed and no
// token is configured), CompareSetupToken always returns false.
func CompareSetupToken(expected, provided string) bool {
	if expected == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}
