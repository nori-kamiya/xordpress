package storage_test

import (
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestOpenAndMigrate_CreatesAllTables(t *testing.T) {
	dataDir := t.TempDir()

	db, err := storage.Open(dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	for _, table := range []string{"users", "posts", "sessions", "settings", "schema_migrations"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrate_IsIdempotentAcrossRestarts(t *testing.T) {
	dataDir := t.TempDir()

	db, err := storage.Open(dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("second Migrate() (simulated restart) error = %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations rows = %d, want 1 (no duplicate re-application)", count)
	}
}
