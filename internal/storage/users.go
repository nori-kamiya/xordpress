// Package storage: users repository.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// User is the persisted representation of a user row.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    int64
}

// CountUsers returns the total number of rows in the users table.
func CountUsers(db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("storage: count users: %w", err)
	}
	return count, nil
}

// GetUserByUsername returns the user with the given username. Returns
// ErrNotFound if no such user exists.
func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	row := db.QueryRow(
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`, username,
	)

	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get user by username: %w", err)
	}
	return u, nil
}
