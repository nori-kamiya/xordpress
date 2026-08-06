// Package storage: login failure tracking (brute-force lockout).
//
// roadmap-to-wp-parity.md P0-3: there was no protection against repeated
// login attempts. State is persisted (not in-memory) so a process restart
// does not hand an attacker a fresh set of attempts.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MaxLoginFailures is the number of consecutive failed attempts against a
// single username that trigger a lockout.
const MaxLoginFailures = 5

// LoginLockoutDuration is how long a username stays locked out once
// MaxLoginFailures is reached.
const LoginLockoutDuration = 15 * time.Minute

// IsLoginLocked reports whether username is currently locked out.
func IsLoginLocked(db *sql.DB, username string) (bool, error) {
	var lockedUntil sql.NullInt64
	err := db.QueryRow(`SELECT locked_until FROM login_failures WHERE username = ?`, username).Scan(&lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("storage: is login locked: %w", err)
	}
	if !lockedUntil.Valid {
		return false, nil
	}
	return time.Now().Unix() < lockedUntil.Int64, nil
}

// RecordLoginFailure increments username's consecutive failure count and, if
// it reaches MaxLoginFailures, locks the account out for LoginLockoutDuration
// from now.
func RecordLoginFailure(db *sql.DB, username string) error {
	now := time.Now().Unix()
	lockedUntil := time.Now().Add(LoginLockoutDuration).Unix()

	_, err := db.Exec(`
		INSERT INTO login_failures (username, fail_count, last_failed_at, locked_until)
		VALUES (?, 1, ?, NULL)
		ON CONFLICT(username) DO UPDATE SET
			fail_count = fail_count + 1,
			last_failed_at = excluded.last_failed_at,
			locked_until = CASE WHEN fail_count + 1 >= ? THEN ? ELSE locked_until END
	`, username, now, MaxLoginFailures, lockedUntil)
	if err != nil {
		return fmt.Errorf("storage: record login failure: %w", err)
	}
	return nil
}

// ResetLoginFailures clears username's failure count and any lockout. Called
// after a successful login.
func ResetLoginFailures(db *sql.DB, username string) error {
	if _, err := db.Exec(`DELETE FROM login_failures WHERE username = ?`, username); err != nil {
		return fmt.Errorf("storage: reset login failures: %w", err)
	}
	return nil
}
