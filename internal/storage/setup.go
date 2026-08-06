// Package storage: first-admin setup.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateFirstAdmin creates the first admin user and records siteName under
// the "site_name" setting, atomically within a single transaction.
//
// It checks the current user count inside the transaction: if no users
// exist, it inserts the admin user and the site_name setting, commits, and
// returns created=true. If a user already exists, it rolls back and returns
// created=false without error. This makes the operation safe to call
// concurrently (e.g. from a first-run setup handler racing with itself) as
// long as the underlying *sql.DB serializes writes (see storage.Open's
// SetMaxOpenConns(1)).
func CreateFirstAdmin(ctx context.Context, db *sql.DB, username, passwordHash, siteName string) (created bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("storage: create first admin: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return false, fmt.Errorf("storage: create first admin: count users: %w", err)
	}

	if count > 0 {
		if rbErr := tx.Rollback(); rbErr != nil {
			return false, fmt.Errorf("storage: create first admin: rollback: %w", rbErr)
		}
		return false, nil
	}

	now := time.Now().Unix()

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, now,
	); err != nil {
		return false, fmt.Errorf("storage: create first admin: insert user: %w", err)
	}

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES ('site_name', ?)`,
		siteName,
	); err != nil {
		return false, fmt.Errorf("storage: create first admin: insert site_name setting: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("storage: create first admin: commit: %w", err)
	}

	return true, nil
}
