// Package storage: settings repository.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// GetSetting returns the value stored under key. Returns ErrNotFound if no
// such setting exists.
func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("storage: get setting: %w", err)
	}
	return value, nil
}

// SetSetting inserts or updates the value stored under key.
func SetSetting(db *sql.DB, key string, value string) error {
	_, err := db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("storage: set setting: %w", err)
	}
	return nil
}
