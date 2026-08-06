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

// InsertSession inserts a new session row for the given tokenHash/userID,
// expiring at expiresAt. created_at is set to the current time.
func InsertSession(db *sql.DB, tokenHash string, userID int64, expiresAt int64) error {
	_, err := db.Exec(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, time.Now().Unix(), expiresAt,
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
