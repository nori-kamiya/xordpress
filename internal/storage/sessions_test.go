package storage_test

import (
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestInsertAndGetSessionRoundTrip(t *testing.T) {
	db := newTestDB(t)

	if err := storage.InsertSession(db, "tokenhash1", 1, 9999999999, "", ""); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	s, err := storage.GetSessionByTokenHash(db, "tokenhash1")
	if err != nil {
		t.Fatalf("GetSessionByTokenHash: %v", err)
	}
	if s.TokenHash != "tokenhash1" || s.UserID != 1 || s.ExpiresAt != 9999999999 {
		t.Fatalf("unexpected session: %+v", s)
	}
}

func TestGetSessionByTokenHashNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := storage.GetSessionByTokenHash(db, "missing")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	db := newTestDB(t)

	if err := storage.InsertSession(db, "tokenhash2", 1, 9999999999, "", ""); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	if err := storage.DeleteSession(db, "tokenhash2"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := storage.GetSessionByTokenHash(db, "tokenhash2")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteSessionNonexistentIsNoop(t *testing.T) {
	db := newTestDB(t)

	if err := storage.DeleteSession(db, "never-existed"); err != nil {
		t.Fatalf("DeleteSession on missing row should not error, got %v", err)
	}
}
