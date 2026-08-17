package storage_test

import (
	"database/sql"
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

	for _, table := range []string{"users", "posts", "sessions", "settings", "login_failures", "schema_migrations", "user_totp", "mfa_recovery_codes", "mfa_challenges", "mfa_failures", "media"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	var role string
	if err := db.QueryRow(`SELECT role FROM users LIMIT 1`).Scan(&role); err != nil && err != sql.ErrNoRows {
		t.Errorf("query role column: %v", err)
	}

	var userAgent, createdIP sql.NullString
	if err := db.QueryRow(`SELECT user_agent, created_ip FROM sessions LIMIT 1`).Scan(&userAgent, &createdIP); err != nil && err != sql.ErrNoRows {
		t.Errorf("query sessions meta columns: %v", err)
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

func TestMigrate_BackfillsExistingUserToAdministrator(t *testing.T) {
	dataDir := t.TempDir()

	db, err := storage.Open(dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		"legacy-admin", "hash", 0,
	); err != nil {
		t.Fatalf("insert legacy user (no role column): %v", err)
	}

	var role string
	if err := db.QueryRow(`SELECT role FROM users WHERE username = ?`, "legacy-admin").Scan(&role); err != nil {
		t.Fatalf("query role: %v", err)
	}
	if role != "administrator" {
		t.Errorf("role = %q, want %q (DEFAULT must be administrator, not contributor — see 0003_user_roles.sql comment)", role, "administrator")
	}
}

func TestMigrate_MFAChallengesEnforcesUserForeignKey(t *testing.T) {
	dataDir := t.TempDir()

	db, err := storage.Open(dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	const nonExistentUserID = 999
	_, err = db.Exec(
		`INSERT INTO mfa_challenges (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		"deadbeef", nonExistentUserID, 0,
	)
	if err == nil {
		t.Fatal("expected a foreign key violation inserting mfa_challenges with a non-existent user_id, got nil error")
	}
}
