// Package storage: media repository.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Media is the persisted representation of a media row. Width/Height are
// nullable because the schema allows for future non-image media types;
// all currently supported types are images, so they are always set on
// insert.
type Media struct {
	ID           int64
	StoredName   string
	OriginalName string
	MimeType     string
	SizeBytes    int64
	Width        sql.NullInt64
	Height       sql.NullInt64
	UploaderID   int64
	CreatedAt    int64
}

// InsertMedia inserts a media row (with created_at set to now) and
// returns its newly assigned id. Callers that need the created_at value
// (e.g. to echo it in an API response) should re-fetch via
// GetMediaByID rather than computing their own time.Now(), to avoid
// returning a value that differs from what was actually stored.
func InsertMedia(db *sql.DB, storedName, originalName, mimeType string, sizeBytes int64, width, height int, uploaderID int64) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO media (stored_name, original_name, mime_type, size_bytes, width, height, uploader_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		storedName, originalName, mimeType, sizeBytes, width, height, uploaderID, time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("storage: insert media: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("storage: insert media: %w", err)
	}
	return id, nil
}

// GetMediaByID returns the media row with the given id. Returns
// ErrNotFound if no such row exists.
func GetMediaByID(db *sql.DB, id int64) (*Media, error) {
	row := db.QueryRow(
		`SELECT id, stored_name, original_name, mime_type, size_bytes, width, height, uploader_id, created_at
		 FROM media WHERE id = ?`, id,
	)
	return scanMedia(row)
}

// ListMedia returns up to limit media rows ordered by created_at DESC, id
// DESC, along with the total row count (ignoring limit/offset), for
// pagination. limit <= 0 means no limit (all rows).
func ListMedia(db *sql.DB, limit, offset int) ([]Media, int, error) {
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("storage: count media: %w", err)
	}

	query := `SELECT id, stored_name, original_name, mime_type, size_bytes, width, height, uploader_id, created_at
		 FROM media ORDER BY created_at DESC, id DESC`
	args := []any{}
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("storage: list media: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only, nothing to recover

	var items []Media
	for rows.Next() {
		m := Media{}
		if err := rows.Scan(&m.ID, &m.StoredName, &m.OriginalName, &m.MimeType, &m.SizeBytes, &m.Width, &m.Height, &m.UploaderID, &m.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("storage: scan media: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("storage: list media: %w", err)
	}
	return items, total, nil
}

// DeleteMedia deletes the media row with the given id. Returns
// ErrNotFound if no such row exists.
func DeleteMedia(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM media WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("storage: delete media: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: delete media: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanMedia scans a single-row query result into a Media, mapping
// sql.ErrNoRows to ErrNotFound.
func scanMedia(row *sql.Row) (*Media, error) {
	m := &Media{}
	err := row.Scan(&m.ID, &m.StoredName, &m.OriginalName, &m.MimeType, &m.SizeBytes, &m.Width, &m.Height, &m.UploaderID, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get media: %w", err)
	}
	return m, nil
}
