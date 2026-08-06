package content_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// newEmptyTestDB returns a migrated in-memory SQLite DB with no seeded
// users, for exercising CreateFirstAdmin's own user-count check.
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

func TestCreateFirstAdminSucceedsWhenNoUsers(t *testing.T) {
	db := newEmptyTestDB(t)

	created, err := content.CreateFirstAdmin(context.Background(), db, "admin", "hash", "xordpress")
	if err != nil {
		t.Fatalf("CreateFirstAdmin: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true")
	}

	got, err := content.GetSiteSettings(db)
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if got.Name != "xordpress" {
		t.Fatalf("expected site name %q, got %q", "xordpress", got.Name)
	}
}

func TestCreateFirstAdminNoOpWhenUserExists(t *testing.T) {
	db := newEmptyTestDB(t)

	created, err := content.CreateFirstAdmin(context.Background(), db, "admin", "hash", "xordpress")
	if err != nil {
		t.Fatalf("CreateFirstAdmin (first): %v", err)
	}
	if !created {
		t.Fatalf("expected created=true on first call")
	}

	created, err = content.CreateFirstAdmin(context.Background(), db, "another", "hash2", "other-name")
	if err != nil {
		t.Fatalf("CreateFirstAdmin (second): %v", err)
	}
	if created {
		t.Fatalf("expected created=false on second call")
	}

	got, err := content.GetSiteSettings(db)
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if got.Name != "xordpress" {
		t.Fatalf("expected site name unchanged at %q, got %q", "xordpress", got.Name)
	}
}
