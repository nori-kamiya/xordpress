// Package content implements the post lifecycle (Create/Update/Delete/
// List/Get) on top of internal/storage. It owns the published_at
// transition rule (design.md §3) and propagates storage's typed errors
// (ErrSlugConflict/ErrNotFound/ErrVersionMismatch) unchanged so callers can
// keep using errors.Is against the storage sentinels.
//
// Format validation (length limits, regexes, etc.) is out of scope here —
// that is the adminapi layer's responsibility.
package content

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// Post is the domain representation of a blog post.
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
	PublishedAt *int64 // nil until the post has been published for the first time
}

// CreateInput carries the fields needed to create a new post.
type CreateInput struct {
	Slug     string
	Title    string
	Body     string
	Status   string
	AuthorID int64
}

// UpdateInput carries the mutable fields for updating an existing post.
type UpdateInput struct {
	Slug   string
	Title  string
	Body   string
	Status string
}

// Create creates a new post. A brand-new post has no prior published_at
// to preserve, so it is treated as transitioning from "draft": if
// in.Status is "published", published_at is set to now; otherwise it
// stays NULL. See resolvePublishedAt for the shared rule (design.md §3).
func Create(db *sql.DB, in CreateInput) (*Post, error) {
	now := time.Now().Unix()
	p := &storage.Post{
		Slug:        in.Slug,
		Title:       in.Title,
		Body:        in.Body,
		Status:      in.Status,
		Version:     1,
		AuthorID:    in.AuthorID,
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishedAt: resolvePublishedAt("draft", in.Status, sql.NullInt64{}, now),
	}
	if err := storage.InsertPost(db, p); err != nil {
		return nil, fmt.Errorf("content: create post: %w", err)
	}
	return fromStorage(p), nil
}

// Update updates the post identified by id, enforcing optimistic
// concurrency via expectedVersion. published_at follows the same
// transition rule as Create: it is set only the first time status moves
// draft -> published, and it is never cleared when moving back to draft.
func Update(db *sql.DB, id int64, expectedVersion int64, in UpdateInput) (*Post, error) {
	current, err := storage.GetPostByID(db, id)
	if err != nil {
		return nil, fmt.Errorf("content: update post: %w", err)
	}

	publishedAt := resolvePublishedAt(current.Status, in.Status, current.PublishedAt, time.Now().Unix())

	if err := storage.UpdatePost(db, id, expectedVersion, storage.PostUpdate{
		Title:       in.Title,
		Slug:        in.Slug,
		Body:        in.Body,
		Status:      in.Status,
		PublishedAt: publishedAt,
	}); err != nil {
		return nil, fmt.Errorf("content: update post: %w", err)
	}

	updated, err := storage.GetPostByID(db, id)
	if err != nil {
		return nil, fmt.Errorf("content: update post: reload: %w", err)
	}
	return fromStorage(updated), nil
}

// Delete deletes the post identified by id, enforcing optimistic
// concurrency via expectedVersion.
func Delete(db *sql.DB, id int64, expectedVersion int64) error {
	if err := storage.DeletePost(db, id, expectedVersion); err != nil {
		return fmt.Errorf("content: delete post: %w", err)
	}
	return nil
}

// Get returns the post with the given id, regardless of status.
// Admin-API only — see design.md §3 on the public/admin read split.
func Get(db *sql.DB, id int64) (*Post, error) {
	p, err := storage.GetPostByID(db, id)
	if err != nil {
		return nil, fmt.Errorf("content: get post: %w", err)
	}
	return fromStorage(p), nil
}

// List returns posts regardless of status. Admin-API only — see
// design.md §3 on the public/admin read split.
func List(db *sql.DB, filter storage.PostFilter) ([]*Post, error) {
	posts, err := storage.ListPosts(db, filter)
	if err != nil {
		return nil, fmt.Errorf("content: list posts: %w", err)
	}
	out := make([]*Post, len(posts))
	for i, p := range posts {
		out[i] = fromStorage(p)
	}
	return out, nil
}

// resolvePublishedAt implements the single published_at transition rule
// (design.md §3): it is set to now only at the moment status moves from
// draft to published AND the current value is NULL (i.e. this is the
// first time the post is published). Any other transition — already
// published, or moving back to draft — leaves the existing value
// untouched.
func resolvePublishedAt(prevStatus, newStatus string, prevPublishedAt sql.NullInt64, now int64) sql.NullInt64 {
	if prevStatus == "draft" && newStatus == "published" && !prevPublishedAt.Valid {
		return sql.NullInt64{Int64: now, Valid: true}
	}
	return prevPublishedAt
}

// fromStorage maps a storage.Post row into the domain Post type.
func fromStorage(p *storage.Post) *Post {
	out := &Post{
		ID:        p.ID,
		Slug:      p.Slug,
		Title:     p.Title,
		Body:      p.Body,
		Status:    p.Status,
		Version:   p.Version,
		AuthorID:  p.AuthorID,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
	if p.PublishedAt.Valid {
		v := p.PublishedAt.Int64
		out.PublishedAt = &v
	}
	return out
}
