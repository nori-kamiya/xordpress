package storage_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// newEmptyTestDB returns a fresh, migrated database with no seeded users,
// suitable for exercising CreateFirstAdmin's "no admin yet" path.
func newEmptyTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

func TestCreateFirstAdmin(t *testing.T) {
	db := newEmptyTestDB(t)

	created, err := storage.CreateFirstAdmin(context.Background(), db, "admin", "hash", "xordpress")
	if err != nil {
		t.Fatalf("CreateFirstAdmin: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true, got false")
	}

	count, err := storage.CountUsers(db)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user after CreateFirstAdmin, got %d", count)
	}

	siteName, err := storage.GetSetting(db, "site_name")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if siteName != "xordpress" {
		t.Fatalf("expected site_name %q, got %q", "xordpress", siteName)
	}
}

func TestCreateFirstAdminNoOpWhenUserExists(t *testing.T) {
	db := newEmptyTestDB(t)

	created, err := storage.CreateFirstAdmin(context.Background(), db, "admin", "hash", "xordpress")
	if err != nil {
		t.Fatalf("CreateFirstAdmin (first): %v", err)
	}
	if !created {
		t.Fatalf("expected first call created=true")
	}

	created, err = storage.CreateFirstAdmin(context.Background(), db, "someone-else", "hash2", "other-site")
	if err != nil {
		t.Fatalf("CreateFirstAdmin (second): %v", err)
	}
	if created {
		t.Fatalf("expected second call created=false")
	}

	count, err := storage.CountUsers(db)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected still 1 user, got %d", count)
	}

	siteName, err := storage.GetSetting(db, "site_name")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if siteName != "xordpress" {
		t.Fatalf("expected original site_name to be unchanged, got %q", siteName)
	}
}

// TestCreateFirstAdminConcurrent fires two goroutines at CreateFirstAdmin
// concurrently against the same *sql.DB. Because the DB uses
// SetMaxOpenConns(1), writes are serialized on the Go side, so exactly one
// call must observe created=true and the other created=false — never both
// true (a duplicate admin) and never both false (no admin created at all).
func TestCreateFirstAdminConcurrent(t *testing.T) {
	db := newEmptyTestDB(t)

	const n = 2
	var wg sync.WaitGroup
	results := make([]bool, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = storage.CreateFirstAdmin(
				context.Background(), db, "admin", "hash", "xordpress",
			)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("CreateFirstAdmin goroutine %d: %v", i, err)
		}
	}

	createdCount := 0
	for _, c := range results {
		if c {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("expected exactly 1 goroutine to report created=true, got %d (results=%v)", createdCount, results)
	}

	count, err := storage.CountUsers(db)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 user after concurrent CreateFirstAdmin calls, got %d", count)
	}
}
