package storage_test

import (
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestIsLoginLocked_NoFailures_ReturnsFalse(t *testing.T) {
	db := newTestDB(t)

	locked, err := storage.IsLoginLocked(db, "admin")
	if err != nil {
		t.Fatalf("IsLoginLocked: %v", err)
	}
	if locked {
		t.Error("expected not locked with no recorded failures")
	}
}

func TestRecordLoginFailure_BelowThreshold_DoesNotLock(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < storage.MaxLoginFailures-1; i++ {
		if err := storage.RecordLoginFailure(db, "admin"); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
	}

	locked, err := storage.IsLoginLocked(db, "admin")
	if err != nil {
		t.Fatalf("IsLoginLocked: %v", err)
	}
	if locked {
		t.Errorf("expected not locked after %d failures (threshold is %d)", storage.MaxLoginFailures-1, storage.MaxLoginFailures)
	}
}

func TestRecordLoginFailure_AtThreshold_Locks(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < storage.MaxLoginFailures; i++ {
		if err := storage.RecordLoginFailure(db, "admin"); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
	}

	locked, err := storage.IsLoginLocked(db, "admin")
	if err != nil {
		t.Fatalf("IsLoginLocked: %v", err)
	}
	if !locked {
		t.Errorf("expected locked after %d failures", storage.MaxLoginFailures)
	}
}

func TestRecordLoginFailure_IsPerUsername(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < storage.MaxLoginFailures; i++ {
		if err := storage.RecordLoginFailure(db, "admin"); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
	}

	locked, err := storage.IsLoginLocked(db, "someone-else")
	if err != nil {
		t.Fatalf("IsLoginLocked: %v", err)
	}
	if locked {
		t.Error("a different username's failures must not lock this one out")
	}
}

func TestResetLoginFailures_ClearsLockout(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < storage.MaxLoginFailures; i++ {
		if err := storage.RecordLoginFailure(db, "admin"); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
	}
	if locked, err := storage.IsLoginLocked(db, "admin"); err != nil || !locked {
		t.Fatalf("precondition failed: expected locked, locked=%v err=%v", locked, err)
	}

	if err := storage.ResetLoginFailures(db, "admin"); err != nil {
		t.Fatalf("ResetLoginFailures: %v", err)
	}

	locked, err := storage.IsLoginLocked(db, "admin")
	if err != nil {
		t.Fatalf("IsLoginLocked: %v", err)
	}
	if locked {
		t.Error("expected not locked after ResetLoginFailures")
	}
}

func TestResetLoginFailures_NonexistentUsernameIsNoop(t *testing.T) {
	db := newTestDB(t)

	if err := storage.ResetLoginFailures(db, "never-recorded"); err != nil {
		t.Fatalf("ResetLoginFailures on a username with no rows should be a no-op, got: %v", err)
	}
}
