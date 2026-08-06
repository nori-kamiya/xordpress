// Package storage: posts repository.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
)

// sqliteConstraintUnique is the modernc.org/sqlite extended error code for
// SQLITE_CONSTRAINT_UNIQUE (see https://sqlite.org/rescode.html). It is
// returned when an INSERT/UPDATE violates the posts.slug unique index.
const sqliteConstraintUnique = 2067

// Sentinel errors returned by the posts repository. Callers (content
// package and above) are expected to compare against these with
// errors.Is.
var (
	ErrNotFound        = errors.New("storage: not found")
	ErrSlugConflict    = errors.New("storage: slug conflict")
	ErrVersionMismatch = errors.New("storage: version mismatch")
)

// Post is the persisted representation of a post row.
type Post struct {
	ID          int64
	Slug        string
	Title       string
	Body        string
	Status      string
	Version     int64
	AuthorID    int64
	CreatedAt   int64
	UpdatedAt   int64
	PublishedAt sql.NullInt64
}

// PostFilter controls pagination for ListPosts. A zero-value PostFilter
// (Limit <= 0) returns all posts, regardless of status.
type PostFilter struct {
	Limit  int
	Offset int
}

// PostUpdate carries the mutable fields for UpdatePost. PublishedAt is
// passed through as-is (the content package is responsible for deciding
// whether/when it transitions away from NULL).
type PostUpdate struct {
	Title       string
	Slug        string
	Body        string
	Status      string
	PublishedAt sql.NullInt64
}

// InsertPost inserts p and sets p.ID to the newly assigned row id on
// success. p.CreatedAt/UpdatedAt/Version are taken as given by the caller.
func InsertPost(db *sql.DB, p *Post) error {
	res, err := db.Exec(
		`INSERT INTO posts (slug, title, body, status, version, author_id, created_at, updated_at, published_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Slug, p.Title, p.Body, p.Status, p.Version, p.AuthorID, p.CreatedAt, p.UpdatedAt, p.PublishedAt,
	)
	if err != nil {
		return mapSlugConflict(err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("storage: insert post: %w", err)
	}
	p.ID = id
	return nil
}

// GetPostByID returns the post with the given id, regardless of status.
// Returns ErrNotFound if no such post exists.
func GetPostByID(db *sql.DB, id int64) (*Post, error) {
	row := db.QueryRow(
		`SELECT id, slug, title, body, status, version, author_id, created_at, updated_at, published_at
		 FROM posts WHERE id = ?`, id,
	)
	return scanPost(row)
}

// GetPostBySlug returns the post with the given slug, regardless of
// status. Returns ErrNotFound if no such post exists.
func GetPostBySlug(db *sql.DB, slug string) (*Post, error) {
	row := db.QueryRow(
		`SELECT id, slug, title, body, status, version, author_id, created_at, updated_at, published_at
		 FROM posts WHERE slug = ?`, slug,
	)
	return scanPost(row)
}

// ListPosts returns posts regardless of status, ordered by published_at
// DESC, id DESC. filter.Limit <= 0 means no limit (all rows).
func ListPosts(db *sql.DB, filter PostFilter) ([]*Post, error) {
	query := `SELECT id, slug, title, body, status, version, author_id, created_at, updated_at, published_at
		 FROM posts ORDER BY published_at DESC, id DESC`
	args := []any{}
	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.Offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list posts: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only, nothing to recover

	var posts []*Post
	for rows.Next() {
		p := &Post{}
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &p.Status, &p.Version, &p.AuthorID, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt); err != nil {
			return nil, fmt.Errorf("storage: scan post: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list posts: %w", err)
	}
	return posts, nil
}

// ListPublished returns up to limit published posts, ordered by
// published_at DESC, id DESC.
func ListPublished(db *sql.DB, limit int) ([]*Post, error) {
	rows, err := db.Query(
		`SELECT id, slug, title, body, status, version, author_id, created_at, updated_at, published_at
		 FROM posts WHERE status = 'published' ORDER BY published_at DESC, id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list published posts: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only, nothing to recover

	var posts []*Post
	for rows.Next() {
		p := &Post{}
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &p.Status, &p.Version, &p.AuthorID, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt); err != nil {
			return nil, fmt.Errorf("storage: scan post: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list published posts: %w", err)
	}
	return posts, nil
}

// GetPublishedBySlug returns the post with the given slug, but only if its
// status is 'published'. Returns ErrNotFound otherwise (including when the
// slug exists but is a draft).
func GetPublishedBySlug(db *sql.DB, slug string) (*Post, error) {
	row := db.QueryRow(
		`SELECT id, slug, title, body, status, version, author_id, created_at, updated_at, published_at
		 FROM posts WHERE slug = ? AND status = 'published'`, slug,
	)
	return scanPost(row)
}

// UpdatePost updates the post with the given id, only if its current
// version matches expectedVersion. On success, version is incremented by
// 1 and updated_at is set to now. Returns ErrVersionMismatch if no row
// matched (whether because the id doesn't exist or the version didn't
// match), and ErrSlugConflict if fields.Slug collides with another post.
func UpdatePost(db *sql.DB, id int64, expectedVersion int64, fields PostUpdate) error {
	res, err := db.Exec(
		`UPDATE posts SET slug = ?, title = ?, body = ?, status = ?, published_at = ?, version = version + 1, updated_at = ?
		 WHERE id = ? AND version = ?`,
		fields.Slug, fields.Title, fields.Body, fields.Status, fields.PublishedAt, time.Now().Unix(), id, expectedVersion,
	)
	if err != nil {
		return mapSlugConflict(err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: update post: %w", err)
	}
	if n == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// DeletePost deletes the post with the given id, only if its current
// version matches expectedVersion. Returns ErrVersionMismatch if no row
// matched (whether because the id doesn't exist or the version didn't
// match).
func DeletePost(db *sql.DB, id int64, expectedVersion int64) error {
	res, err := db.Exec(`DELETE FROM posts WHERE id = ? AND version = ?`, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("storage: delete post: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: delete post: %w", err)
	}
	if n == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// scanPost scans a single-row query result into a Post, mapping
// sql.ErrNoRows to ErrNotFound.
func scanPost(row *sql.Row) (*Post, error) {
	p := &Post{}
	err := row.Scan(&p.ID, &p.Slug, &p.Title, &p.Body, &p.Status, &p.Version, &p.AuthorID, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get post: %w", err)
	}
	return p, nil
}

// mapSlugConflict inspects err for a SQLITE_CONSTRAINT_UNIQUE violation
// (extended code 2067) and, if found, returns ErrSlugConflict instead. Any
// other error is returned unchanged (wrapped for context).
func mapSlugConflict(err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqliteConstraintUnique {
		return ErrSlugConflict
	}
	return fmt.Errorf("storage: %w", err)
}
