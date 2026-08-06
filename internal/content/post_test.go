package content_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// newTestDB returns a migrated in-memory SQLite DB with one user (id=1)
// to satisfy posts.author_id's foreign key.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, created_at) VALUES (1, 'admin', 'hash', 0)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	return db
}

func TestCreateSlugConflictReturnsTypedError(t *testing.T) {
	db := newTestDB(t)

	in := content.CreateInput{Slug: "dup", Title: "First", Body: "<p>a</p>", Status: "draft", AuthorID: 1}
	if _, err := content.Create(db, in); err != nil {
		t.Fatalf("Create 1: %v", err)
	}

	dup := content.CreateInput{Slug: "dup", Title: "Second", Body: "<p>b</p>", Status: "draft", AuthorID: 1}
	_, err := content.Create(db, dup)
	if !errors.Is(err, storage.ErrSlugConflict) {
		t.Fatalf("expected ErrSlugConflict, got %v", err)
	}
}

func TestUpdateSlugConflictReturnsTypedError(t *testing.T) {
	db := newTestDB(t)

	a, err := content.Create(db, content.CreateInput{Slug: "post-a", Title: "A", Body: "<p>a</p>", Status: "draft", AuthorID: 1})
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	b, err := content.Create(db, content.CreateInput{Slug: "post-b", Title: "B", Body: "<p>b</p>", Status: "draft", AuthorID: 1})
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}

	_, err = content.Update(db, b.ID, b.Version, content.UpdateInput{
		Slug: a.Slug, Title: b.Title, Body: b.Body, Status: b.Status,
	})
	if !errors.Is(err, storage.ErrSlugConflict) {
		t.Fatalf("expected ErrSlugConflict, got %v", err)
	}
}

func TestGetNotFoundReturnsTypedError(t *testing.T) {
	db := newTestDB(t)

	_, err := content.Get(db, 999)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateVersionMismatchReturnsTypedError(t *testing.T) {
	db := newTestDB(t)

	p, err := content.Create(db, content.CreateInput{Slug: "post", Title: "T", Body: "<p>b</p>", Status: "draft", AuthorID: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = content.Update(db, p.ID, p.Version+1, content.UpdateInput{
		Slug: p.Slug, Title: p.Title, Body: p.Body, Status: p.Status,
	})
	if !errors.Is(err, storage.ErrVersionMismatch) {
		t.Fatalf("expected ErrVersionMismatch, got %v", err)
	}
}

func TestDeleteVersionMismatchReturnsTypedError(t *testing.T) {
	db := newTestDB(t)

	p, err := content.Create(db, content.CreateInput{Slug: "post", Title: "T", Body: "<p>b</p>", Status: "draft", AuthorID: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = content.Delete(db, p.ID, p.Version+1)
	if !errors.Is(err, storage.ErrVersionMismatch) {
		t.Fatalf("expected ErrVersionMismatch, got %v", err)
	}
}

func TestPublishedAtSetOnlyOnFirstPublish(t *testing.T) {
	db := newTestDB(t)

	// Creating directly as published sets published_at.
	p, err := content.Create(db, content.CreateInput{Slug: "post", Title: "T", Body: "<p>b</p>", Status: "published", AuthorID: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.PublishedAt == nil {
		t.Fatalf("expected PublishedAt to be set on first publish, got nil")
	}
	firstPublishedAt := *p.PublishedAt

	// Updating while staying published must not change published_at
	// (it stays the original first-publish timestamp, not overwritten
	// even though the DB row is re-saved).
	updated, err := content.Update(db, p.ID, p.Version, content.UpdateInput{
		Slug: p.Slug, Title: "Updated title", Body: p.Body, Status: "published",
	})
	if err != nil {
		t.Fatalf("Update (stay published): %v", err)
	}
	if updated.PublishedAt == nil || *updated.PublishedAt != firstPublishedAt {
		t.Fatalf("expected published_at to remain %d, got %v", firstPublishedAt, updated.PublishedAt)
	}
}

func TestPublishedAtNotClearedWhenReturnedToDraft(t *testing.T) {
	db := newTestDB(t)

	p, err := content.Create(db, content.CreateInput{Slug: "post", Title: "T", Body: "<p>b</p>", Status: "draft", AuthorID: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.PublishedAt != nil {
		t.Fatalf("expected PublishedAt nil for draft, got %v", p.PublishedAt)
	}

	published, err := content.Update(db, p.ID, p.Version, content.UpdateInput{
		Slug: p.Slug, Title: p.Title, Body: p.Body, Status: "published",
	})
	if err != nil {
		t.Fatalf("Update (publish): %v", err)
	}
	if published.PublishedAt == nil {
		t.Fatalf("expected PublishedAt to be set after first publish")
	}
	firstPublishedAt := *published.PublishedAt

	backToDraft, err := content.Update(db, published.ID, published.Version, content.UpdateInput{
		Slug: published.Slug, Title: published.Title, Body: published.Body, Status: "draft",
	})
	if err != nil {
		t.Fatalf("Update (back to draft): %v", err)
	}
	if backToDraft.PublishedAt == nil || *backToDraft.PublishedAt != firstPublishedAt {
		t.Fatalf("expected published_at to remain %d after reverting to draft, got %v", firstPublishedAt, backToDraft.PublishedAt)
	}
	if backToDraft.Status != "draft" {
		t.Fatalf("expected status draft, got %q", backToDraft.Status)
	}
}

func TestListReturnsAllRegardlessOfStatus(t *testing.T) {
	db := newTestDB(t)

	if _, err := content.Create(db, content.CreateInput{Slug: "draft-post", Title: "D", Body: "<p>d</p>", Status: "draft", AuthorID: 1}); err != nil {
		t.Fatalf("Create draft: %v", err)
	}
	if _, err := content.Create(db, content.CreateInput{Slug: "published-post", Title: "P", Body: "<p>p</p>", Status: "published", AuthorID: 1}); err != nil {
		t.Fatalf("Create published: %v", err)
	}

	posts, err := content.List(db, storage.PostFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts (draft + published), got %d", len(posts))
	}
}
