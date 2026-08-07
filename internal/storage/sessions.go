// Package storage: sessions repository.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Session is the persisted representation of a session row.
type Session struct {
	TokenHash string
	UserID    int64
	CreatedAt int64
	ExpiresAt int64
}

// SessionSummary is a session row shaped for the session-listing UI
// (design.md §5, GET /api/account/sessions). ID is the first 16 hex
// characters of the session's token_hash rather than the hash itself, so
// the full hash (equivalent to the raw session token) is never exposed to
// the client that owns the session (design.md §5's id-scheme rationale).
type SessionSummary struct {
	ID        string
	CreatedAt int64
	ExpiresAt int64
	UserAgent string
	CreatedIP string
}

// ListSessionsByUserID returns every non-expired session belonging to
// userID, most recently created first. Expired rows are excluded (matching
// ValidateSession's expiry check) rather than purged, since nothing else in
// this package deletes on read; callers should not have to filter dead
// sessions out of the response themselves.
func ListSessionsByUserID(db *sql.DB, userID int64) ([]SessionSummary, error) {
	rows, err := db.Query(
		`SELECT substr(token_hash, 1, 16), created_at, expires_at, user_agent, created_ip
		 FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC`,
		userID, time.Now().Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list sessions by user id: %w", err)
	}
	defer rows.Close()

	var sessions []SessionSummary
	for rows.Next() {
		var s SessionSummary
		var userAgent, createdIP sql.NullString
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.ExpiresAt, &userAgent, &createdIP); err != nil {
			return nil, fmt.Errorf("storage: list sessions by user id: scan: %w", err)
		}
		s.UserAgent = userAgent.String
		s.CreatedIP = createdIP.String
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list sessions by user id: %w", err)
	}
	return sessions, nil
}

// SessionOwnerByPrefix returns the user_id of the session whose token_hash's
// first 16 hex characters equal idPrefix, regardless of which user is
// asking. Returns ErrNotFound if no such session exists. Callers use this to
// distinguish "already deleted" (idempotent success) from "exists but
// belongs to someone else" (design.md §5) before deleting.
func SessionOwnerByPrefix(db *sql.DB, idPrefix string) (int64, error) {
	var userID int64
	err := db.QueryRow(
		`SELECT user_id FROM sessions WHERE substr(token_hash, 1, 16) = ?`, idPrefix,
	).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("storage: session owner by prefix: %w", err)
	}
	return userID, nil
}

// DeleteSessionByPrefix deletes the session belonging to userID whose
// token_hash's first 16 hex characters equal idPrefix (the id scheme
// returned by ListSessionsByUserID). Matching is by exact equality, not
// SQL LIKE, so idPrefix cannot be used to inject wildcard patterns
// (design.md §5). No-op (no error) if no such session exists, so callers
// can treat deletion as idempotent.
func DeleteSessionByPrefix(db *sql.DB, userID int64, idPrefix string) error {
	if _, err := db.Exec(
		`DELETE FROM sessions WHERE user_id = ? AND substr(token_hash, 1, 16) = ?`,
		userID, idPrefix,
	); err != nil {
		return fmt.Errorf("storage: delete session by prefix: %w", err)
	}
	return nil
}

// InsertSession inserts a new session row for the given tokenHash/userID,
// expiring at expiresAt. created_at is set to the current time. userAgent
// and ip are stored as-is for the session listing UI (0004_sessions_meta.sql)
// and may be empty.
func InsertSession(db *sql.DB, tokenHash string, userID int64, expiresAt int64, userAgent, ip string) error {
	_, err := db.Exec(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at, user_agent, created_ip) VALUES (?, ?, ?, ?, ?, ?)`,
		tokenHash, userID, time.Now().Unix(), expiresAt, userAgent, ip,
	)
	if err != nil {
		return fmt.Errorf("storage: insert session: %w", err)
	}
	return nil
}

// GetSessionByTokenHash returns the session with the given token hash.
// Returns ErrNotFound if no such session exists.
func GetSessionByTokenHash(db *sql.DB, tokenHash string) (*Session, error) {
	row := db.QueryRow(
		`SELECT token_hash, user_id, created_at, expires_at FROM sessions WHERE token_hash = ?`, tokenHash,
	)

	s := &Session{}
	err := row.Scan(&s.TokenHash, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get session by token hash: %w", err)
	}
	return s, nil
}

// DeleteSession deletes the session with the given token hash. It is a
// no-op (no error) if no such session exists.
func DeleteSession(db *sql.DB, tokenHash string) error {
	if _, err := db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("storage: delete session: %w", err)
	}
	return nil
}

// DeleteSessionsByUserID deletes every session belonging to userID. Used
// after a password change to force re-authentication everywhere (including
// the session that made the change), so a stolen/leaked old session token
// stops working the moment the password is rotated. No-op if none exist.
func DeleteSessionsByUserID(db *sql.DB, userID int64) error {
	if _, err := db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("storage: delete sessions by user id: %w", err)
	}
	return nil
}
