package auth

import (
	"context"
	"testing"
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
