// Package storage: 2FA challenge failure tracking (brute-force lockout on
// TOTP/recovery-code verification), keyed by user_id rather than username
// (design.md §6.1). Structurally the same as loginfailures.go's UPSERT
// logic, but a fully independent table: login_failures (checked by
// auth.Authenticate before a 2FA challenge is even issued) and
// mfa_failures (checked here) never influence each other.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MaxMFAFailures is the number of consecutive failed 2FA verification
// attempts against a single user that trigger a lockout (design.md §6.1).
const MaxMFAFailures = 10

// MFALockoutDuration is how long a user stays locked out of 2FA
// verification once MaxMFAFailures is reached.
const MFALockoutDuration = 15 * time.Minute

// IsMFALocked reports whether userID is currently locked out of 2FA
// verification.
func IsMFALocked(db *sql.DB, userID int64) (bool, error) {
	var lockedUntil sql.NullInt64
	err := db.QueryRow(`SELECT locked_until FROM mfa_failures WHERE user_id = ?`, userID).Scan(&lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("storage: is mfa locked: %w", err)
	}
	if !lockedUntil.Valid {
		return false, nil
	}
	return time.Now().Unix() < lockedUntil.Int64, nil
}

// RecordMFAFailure increments userID's consecutive 2FA failure count and,
// if it reaches MaxMFAFailures, locks the account out of 2FA verification
// for MFALockoutDuration from now.
func RecordMFAFailure(db *sql.DB, userID int64) error {
	now := time.Now().Unix()
	lockedUntil := time.Now().Add(MFALockoutDuration).Unix()

	_, err := db.Exec(`
		INSERT INTO mfa_failures (user_id, fail_count, last_failed_at, locked_until)
		VALUES (?, 1, ?, NULL)
		ON CONFLICT(user_id) DO UPDATE SET
			fail_count = fail_count + 1,
			last_failed_at = excluded.last_failed_at,
			locked_until = CASE WHEN fail_count + 1 >= ? THEN ? ELSE locked_until END
	`, userID, now, MaxMFAFailures, lockedUntil)
	if err != nil {
		return fmt.Errorf("storage: record mfa failure: %w", err)
	}
	return nil
}

// ResetMFAFailures clears userID's 2FA failure count and any lockout.
// Called after a successful 2FA verification.
func ResetMFAFailures(db *sql.DB, userID int64) error {
	if _, err := db.Exec(`DELETE FROM mfa_failures WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("storage: reset mfa failures: %w", err)
	}
	return nil
}
