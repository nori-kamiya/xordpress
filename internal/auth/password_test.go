package auth

import "testing"

func TestHashPassword_RoundTripSucceeds(t *testing.T) {
	plain := "correct horse battery staple"

	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	if !VerifyPassword(hash, plain) {
		t.Fatal("VerifyPassword returned false for the correct password")
	}
}

func TestVerifyPassword_WrongPasswordFails(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	if VerifyPassword(hash, "wrong password") {
		t.Fatal("VerifyPassword returned true for an incorrect password")
	}
}

func TestHashPassword_LongPlaintextDoesNotError(t *testing.T) {
	// 100 bytes, well over bcrypt's 72-byte input limit. Because HashPassword
	// pre-hashes with SHA-256 and hex-encodes (always 64 chars) before
	// calling bcrypt, this must succeed without error.
	longPlain := ""
	for i := 0; i < 100; i++ {
		longPlain += "a"
	}

	hash, err := HashPassword(longPlain)
	if err != nil {
		t.Fatalf("HashPassword returned error for long plaintext: %v", err)
	}

	if !VerifyPassword(hash, longPlain) {
		t.Fatal("VerifyPassword returned false for the correct long password")
	}
}
