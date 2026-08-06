package storage_test

import (
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestCountUsers(t *testing.T) {
	db := newTestDB(t)

	count, err := storage.CountUsers(db)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user (seeded by newTestDB), got %d", count)
	}
}

func TestGetUserByUsername(t *testing.T) {
	db := newTestDB(t)

	u, err := storage.GetUserByUsername(db, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if u.ID != 1 || u.Username != "admin" || u.PasswordHash != "hash" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestGetUserByUsernameNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := storage.GetUserByUsername(db, "nobody")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
