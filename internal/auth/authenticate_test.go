package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestAuthenticate_ValidCredentials(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Create a user with a known password
	plain := "correct horse battery staple"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Insert user with hashed password
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		2, "testuser", hash, 0,
	); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	userID, ok, err := Authenticate(ctx, db, "testuser", plain)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if !ok {
		t.Fatal("Authenticate returned ok=false for valid credentials")
	}
	if userID != 2 {
		t.Errorf("userID = %d, want 2", userID)
	}
}

func TestAuthenticate_NonExistentUsername(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID, ok, err := Authenticate(ctx, db, "nonexistent", "anypassword")
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if ok {
		t.Fatal("Authenticate returned ok=true for non-existent username")
	}
	if userID != 0 {
		t.Errorf("userID = %d, want 0", userID)
	}
}

func TestAuthenticate_IncorrectPassword(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Create a user with a known password
	plain := "correct horse battery staple"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Insert user with hashed password
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		3, "testuser", hash, 0,
	); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	userID, ok, err := Authenticate(ctx, db, "testuser", "wrong password")
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if ok {
		t.Fatal("Authenticate returned ok=true for incorrect password")
	}
	if userID != 0 {
		t.Errorf("userID = %d, want 0", userID)
	}
}

// Regression test for roadmap-to-wp-parity.md P0-3 (brute-force lockout).
func TestAuthenticate_LocksOutAfterRepeatedFailures(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	plain := "correct horse battery staple"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		4, "lockout-user", hash, 0,
	); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	for i := 0; i < storage.MaxLoginFailures; i++ {
		if _, ok, err := Authenticate(ctx, db, "lockout-user", "wrong password"); err != nil || ok {
			t.Fatalf("attempt %d: got ok=%v err=%v, want ok=false err=nil", i, ok, err)
		}
	}

	// The threshold-th failure above should have locked the account -- even
	// the *correct* password must now be rejected with ErrAccountLocked.
	userID, ok, err := Authenticate(ctx, db, "lockout-user", plain)
	if ok {
		t.Fatal("Authenticate returned ok=true for a locked-out account")
	}
	if userID != 0 {
		t.Errorf("userID = %d, want 0", userID)
	}
	if !errors.Is(err, ErrAccountLocked) {
		t.Errorf("err = %v, want ErrAccountLocked", err)
	}
}

func TestAuthenticate_SuccessResetsFailureCount(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	plain := "correct horse battery staple"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		5, "reset-user", hash, 0,
	); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	for i := 0; i < storage.MaxLoginFailures-1; i++ {
		if _, ok, err := Authenticate(ctx, db, "reset-user", "wrong password"); err != nil || ok {
			t.Fatalf("attempt %d: got ok=%v err=%v, want ok=false err=nil", i, ok, err)
		}
	}

	// One below the threshold, then a successful login -- must reset the
	// count rather than letting it carry over toward a future lockout.
	if _, ok, err := Authenticate(ctx, db, "reset-user", plain); err != nil || !ok {
		t.Fatalf("successful login: got ok=%v err=%v, want ok=true err=nil", ok, err)
	}

	if _, ok, err := Authenticate(ctx, db, "reset-user", "wrong password"); err != nil || ok {
		t.Fatalf("post-reset attempt: got ok=%v err=%v, want ok=false err=nil (not locked)", ok, err)
	}
}
