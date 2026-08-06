// Package auth provides authentication primitives such as password hashing.
package auth

import (
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plaintext password.
//
// The plaintext is first pre-hashed with SHA-256 and hex-encoded, then the
// resulting 64-character hex string is passed to bcrypt.GenerateFromPassword
// with bcrypt.DefaultCost. Pre-hashing keeps the input to bcrypt at a fixed
// 64 bytes, so passwords longer than bcrypt's 72-byte limit never cause an
// error.
func HashPassword(plain string) (string, error) {
	sum := sha256.Sum256([]byte(plain))
	hexSum := hex.EncodeToString(sum[:])

	hashed, err := bcrypt.GenerateFromPassword([]byte(hexSum), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hashed), nil
}

// VerifyPassword reports whether plain matches hash, which must have been
// produced by HashPassword.
func VerifyPassword(hash, plain string) bool {
	sum := sha256.Sum256([]byte(plain))
	hexSum := hex.EncodeToString(sum[:])

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(hexSum))
	return err == nil
}
