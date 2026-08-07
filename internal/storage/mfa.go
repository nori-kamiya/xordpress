// Package storage: TOTP registration, login challenges, and recovery codes
// (design.md §3, §6.1). mfa_challenges is deliberately its own table rather
// than a flag on sessions, so a half-authenticated login never appears in
// any table auth.ValidateSession reads from.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// UserTOTP is the persisted representation of a user_totp row.
type UserTOTP struct {
	UserID       int64
	Secret       string
	ConfirmedAt  *int64
	LastUsedStep int64
}

// GetUserTOTP returns the TOTP registration for userID. Returns ErrNotFound
// if the user has never started enrollment.
func GetUserTOTP(db *sql.DB, userID int64) (*UserTOTP, error) {
	var t UserTOTP
	var confirmedAt sql.NullInt64
	err := db.QueryRow(
		`SELECT user_id, secret, confirmed_at, last_used_step FROM user_totp WHERE user_id = ?`, userID,
	).Scan(&t.UserID, &t.Secret, &confirmedAt, &t.LastUsedStep)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get user totp: %w", err)
	}
	if confirmedAt.Valid {
		t.ConfirmedAt = &confirmedAt.Int64
	}
	return &t, nil
}

// UpsertUserTOTPSecret starts (or restarts) enrollment for userID: it
// stores secret and resets confirmed_at to NULL (unconfirmed) and
// last_used_step to 0, so a re-enrollment cannot be satisfied by a code
// generated against the old secret.
func UpsertUserTOTPSecret(db *sql.DB, userID int64, secret string) error {
	_, err := db.Exec(`
		INSERT INTO user_totp (user_id, secret, confirmed_at, last_used_step)
		VALUES (?, ?, NULL, 0)
		ON CONFLICT(user_id) DO UPDATE SET secret = excluded.secret, confirmed_at = NULL, last_used_step = 0
	`, userID, secret)
	if err != nil {
		return fmt.Errorf("storage: upsert user totp secret: %w", err)
	}
	return nil
}

// ConfirmUserTOTP marks userID's TOTP enrollment confirmed as of now,
// activating it for login (design.md §6.1: unconfirmed registrations do not
// require 2FA at login). confirmingStep is the TOTP step of the code the
// caller just verified to confirm enrollment; it is stored as
// last_used_step so that same code cannot be replayed at the very next
// login (design.md §6.1 step 3's replay protection, which would otherwise
// have a one-code gap between confirmation and last_used_step's initial 0).
func ConfirmUserTOTP(db *sql.DB, userID, confirmingStep int64) error {
	res, err := db.Exec(
		`UPDATE user_totp SET confirmed_at = ?, last_used_step = ? WHERE user_id = ?`,
		time.Now().Unix(), confirmingStep, userID,
	)
	if err != nil {
		return fmt.Errorf("storage: confirm user totp: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateUserTOTPLastUsedStep records step as the most recently accepted
// TOTP step for userID, so a later Validate call can reject a replay of the
// same or an earlier step (design.md §6.1).
func UpdateUserTOTPLastUsedStep(db *sql.DB, userID, step int64) error {
	if _, err := db.Exec(`UPDATE user_totp SET last_used_step = ? WHERE user_id = ?`, step, userID); err != nil {
		return fmt.Errorf("storage: update user totp last used step: %w", err)
	}
	return nil
}

// DeleteUserTOTP removes userID's TOTP registration entirely (DELETE
// /api/account/mfa, design.md §5).
func DeleteUserTOTP(db *sql.DB, userID int64) error {
	if _, err := db.Exec(`DELETE FROM user_totp WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("storage: delete user totp: %w", err)
	}
	return nil
}

// MFAChallengeTTL is how long a pending 2FA login challenge remains valid
// (design.md §6.1).
const MFAChallengeTTL = 5 * time.Minute

// MFAChallenge is the persisted representation of an mfa_challenges row.
type MFAChallenge struct {
	UserID    int64
	ExpiresAt int64
}

// CreateMFAChallenge deletes any already-expired challenge rows and then
// inserts a new one for userID identified by tokenHash (the SHA-256 of the
// raw mfa_token, matching sessions' token_hash convention), expiring after
// MFAChallengeTTL. Both statements run in one transaction (design.md §6.1)
// so repeated login attempts cannot accumulate unbounded expired rows.
func CreateMFAChallenge(db *sql.DB, tokenHash string, userID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("storage: create mfa challenge: begin: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	if _, err := tx.Exec(`DELETE FROM mfa_challenges WHERE expires_at < ?`, now.Unix()); err != nil {
		return fmt.Errorf("storage: create mfa challenge: clean expired: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO mfa_challenges (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash, userID, now.Add(MFAChallengeTTL).Unix(),
	); err != nil {
		return fmt.Errorf("storage: create mfa challenge: insert: %w", err)
	}
	return tx.Commit()
}

// GetMFAChallenge returns the non-expired challenge identified by
// tokenHash. Returns ErrNotFound if no such row exists or it has expired
// (an expired row is treated as absent, matching sessions' lazy-expiry
// convention — it is not proactively deleted here).
func GetMFAChallenge(db *sql.DB, tokenHash string) (*MFAChallenge, error) {
	var c MFAChallenge
	err := db.QueryRow(
		`SELECT user_id, expires_at FROM mfa_challenges WHERE token_hash = ?`, tokenHash,
	).Scan(&c.UserID, &c.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get mfa challenge: %w", err)
	}
	if time.Now().Unix() > c.ExpiresAt {
		return nil, ErrNotFound
	}
	return &c, nil
}

// DeleteMFAChallenge removes the challenge identified by tokenHash, once
// consumed by a successful or exhausted verification attempt.
func DeleteMFAChallenge(db *sql.DB, tokenHash string) error {
	if _, err := db.Exec(`DELETE FROM mfa_challenges WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("storage: delete mfa challenge: %w", err)
	}
	return nil
}

// InsertRecoveryCodes stores codeHashes (bcrypt hashes) as userID's
// recovery codes, each initially unused.
func InsertRecoveryCodes(db *sql.DB, userID int64, codeHashes []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("storage: insert recovery codes: begin: %w", err)
	}
	defer tx.Rollback()

	for _, hash := range codeHashes {
		if _, err := tx.Exec(
			`INSERT INTO mfa_recovery_codes (user_id, code_hash, used_at) VALUES (?, ?, NULL)`,
			userID, hash,
		); err != nil {
			return fmt.Errorf("storage: insert recovery codes: %w", err)
		}
	}
	return tx.Commit()
}

// UnusedRecoveryCodeHashes returns every not-yet-used recovery code hash
// for userID, for the caller to bcrypt-compare against a submitted code
// (hashes can't be looked up by equality on the plaintext).
func UnusedRecoveryCodeHashes(db *sql.DB, userID int64) ([]string, error) {
	rows, err := db.Query(
		`SELECT code_hash FROM mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: unused recovery code hashes: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("storage: unused recovery code hashes: scan: %w", err)
		}
		hashes = append(hashes, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: unused recovery code hashes: %w", err)
	}
	return hashes, nil
}

// MarkRecoveryCodeUsed sets used_at on the recovery code row identified by
// its exact hash, so it cannot be replayed.
func MarkRecoveryCodeUsed(db *sql.DB, userID int64, codeHash string) error {
	if _, err := db.Exec(
		`UPDATE mfa_recovery_codes SET used_at = ? WHERE user_id = ? AND code_hash = ?`,
		time.Now().Unix(), userID, codeHash,
	); err != nil {
		return fmt.Errorf("storage: mark recovery code used: %w", err)
	}
	return nil
}
