// Package storage provides the SQLite-backed persistence layer.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens (creating if necessary) the SQLite database located under
// dataDir/data/xordpress.db, ensuring the containing directory exists.
//
// The returned *sql.DB is configured with a single-connection pool
// (SetMaxOpenConns(1)) so that SQLite's single-writer constraint is
// serialized on the Go side rather than relying on SQLITE_BUSY retries.
func Open(dataDir string) (*sql.DB, error) {
	dir := filepath.Join(dataDir, "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create data dir: %w", err)
	}

	dbPath := filepath.Join(dir, "xordpress.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate",
		dbPath,
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	return db, nil
}
