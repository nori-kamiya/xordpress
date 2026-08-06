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

	for _, table := range []string{"users", "posts", "sessions", "settings", "login_failures", "schema_migrations"} {
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

	var countAfterFirst int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&countAfterFirst); err != nil {
		t.Fatalf("count schema_migrations after first Migrate: %v", err)
	}
	if countAfterFirst == 0 {
		t.Fatal("schema_migrations has 0 rows after the first Migrate() — expected at least one applied migration")
	}

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("second Migrate() (simulated restart) error = %v", err)
	}

	var countAfterSecond int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&countAfterSecond); err != nil {
		t.Fatalf("count schema_migrations after second Migrate: %v", err)
	}
	// Not hardcoded to the current migration file count -- what this test
	// guards is that re-running Migrate() never re-applies an already-applied
	// file, regardless of how many migration files exist.
	if countAfterSecond != countAfterFirst {
		t.Errorf("schema_migrations rows after second Migrate() = %d, want %d (no duplicate re-application)",
			countAfterSecond, countAfterFirst)
	}
}
