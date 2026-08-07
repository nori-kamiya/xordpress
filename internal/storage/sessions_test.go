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

func TestListSessionsByUserID_ScopedToUserAndOrderedNewestFirst(t *testing.T) {
	db := newTestDB(t)

	if err := storage.InsertSession(db, "aaaaaaaaaaaaaaaaaaaa", 1, 9999999999, "curl/8.0", "127.0.0.1"); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	if err := storage.InsertSession(db, "bbbbbbbbbbbbbbbbbbbb", 1, 9999999998, "", ""); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	if err := storage.InsertSession(db, "cccccccccccccccccccc", 2, 9999999999, "", ""); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	sessions, err := storage.ListSessionsByUserID(db, 1)
	if err != nil {
		t.Fatalf("ListSessionsByUserID: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].ID != "aaaaaaaaaaaaaaaa" {
		t.Errorf("sessions[0].ID = %q, want the newest session first", sessions[0].ID)
	}
	if sessions[0].UserAgent != "curl/8.0" || sessions[0].CreatedIP != "127.0.0.1" {
		t.Errorf("unexpected meta on sessions[0]: %+v", sessions[0])
	}
	if sessions[1].UserAgent != "" || sessions[1].CreatedIP != "" {
		t.Errorf("expected empty strings (not error) for NULL meta columns, got %+v", sessions[1])
	}
}

func TestSessionOwnerByPrefix(t *testing.T) {
	db := newTestDB(t)

	if err := storage.InsertSession(db, "aaaaaaaaaaaaaaaaaaaa", 7, 9999999999, "", ""); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	ownerID, err := storage.SessionOwnerByPrefix(db, "aaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("SessionOwnerByPrefix: %v", err)
	}
	if ownerID != 7 {
		t.Errorf("ownerID = %d, want 7", ownerID)
	}

	if _, err := storage.SessionOwnerByPrefix(db, "0000000000000000"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown prefix, got %v", err)
	}
}

func TestDeleteSessionByPrefix_OnlyDeletesMatchingUser(t *testing.T) {
	db := newTestDB(t)

	if err := storage.InsertSession(db, "aaaaaaaaaaaaaaaaaaaa", 1, 9999999999, "", ""); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	// A prefix match for the wrong user must not delete another user's session.
	if err := storage.DeleteSessionByPrefix(db, 2, "aaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("DeleteSessionByPrefix: %v", err)
	}
	if _, err := storage.GetSessionByTokenHash(db, "aaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("session should still exist: %v", err)
	}

	if err := storage.DeleteSessionByPrefix(db, 1, "aaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("DeleteSessionByPrefix: %v", err)
	}
	if _, err := storage.GetSessionByTokenHash(db, "aaaaaaaaaaaaaaaaaaaa"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
