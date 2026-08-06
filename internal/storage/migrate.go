package storage

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/nori-kamiya/xordpress/internal/storage/migrations"
)

// Migrate applies the embedded SQL migration files to db, in filename order.
//
// It first ensures the schema_migrations bookkeeping table exists (created
// directly by this function, not via an embedded migration file, to avoid a
// bootstrap cycle: the runner would otherwise need schema_migrations to know
// which migrations are applied, before it could apply the migration that
// creates schema_migrations).
//
// Each migration file is applied in its own transaction: BEGIN, execute the
// file's SQL, INSERT INTO schema_migrations, COMMIT. Already-applied files
// are skipped, so Migrate is safe to call repeatedly (e.g. on every process
// restart).
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("storage: create schema_migrations: %w", err)
	}

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("storage: read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for i, name := range names {
		version := i + 1

		var applied int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied); err != nil {
			return fmt.Errorf("storage: check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		sqlBytes, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("storage: read migration %s: %w", name, err)
		}

		if err := applyMigration(db, version, string(sqlBytes)); err != nil {
			return fmt.Errorf("storage: apply migration %s: %w", name, err)
		}
	}

	return nil
}

func applyMigration(db *sql.DB, version int, sqlText string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	if _, err := tx.Exec(sqlText); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%s','now'))`, version); err != nil {
		return fmt.Errorf("record applied: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
