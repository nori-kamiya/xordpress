package storage_test

import (
	"database/sql"
	"errors"
	"testing"

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

func newPost(slug string) *storage.Post {
	return &storage.Post{
		Slug:      slug,
		Title:     "Title for " + slug,
		Body:      "<p>body</p>",
		Status:    "draft",
		Version:   1,
		AuthorID:  1,
		CreatedAt: 1000,
		UpdatedAt: 1000,
	}
}

func TestInsertAndGetPostRoundTrip(t *testing.T) {
	db := newTestDB(t)

	p := newPost("hello-world")
	if err := storage.InsertPost(db, p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	if p.ID == 0 {
		t.Fatalf("expected InsertPost to set ID, got 0")
	}

	byID, err := storage.GetPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID: %v", err)
	}
	if byID.Slug != "hello-world" || byID.Title != p.Title {
		t.Fatalf("GetPostByID returned unexpected post: %+v", byID)
	}

	bySlug, err := storage.GetPostBySlug(db, "hello-world")
	if err != nil {
		t.Fatalf("GetPostBySlug: %v", err)
	}
	if bySlug.ID != p.ID {
		t.Fatalf("GetPostBySlug returned wrong post: %+v", bySlug)
	}
}

func TestGetPostByIDNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := storage.GetPostByID(db, 999)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPostBySlugNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := storage.GetPostBySlug(db, "nope")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestInsertPostSlugConflict(t *testing.T) {
	db := newTestDB(t)

	if err := storage.InsertPost(db, newPost("dup")); err != nil {
		t.Fatalf("InsertPost 1: %v", err)
	}
	err := storage.InsertPost(db, newPost("dup"))
	if !errors.Is(err, storage.ErrSlugConflict) {
		t.Fatalf("expected ErrSlugConflict, got %v", err)
	}
}

func TestUpdatePostSlugConflict(t *testing.T) {
	db := newTestDB(t)

	a := newPost("post-a")
	if err := storage.InsertPost(db, a); err != nil {
		t.Fatalf("InsertPost a: %v", err)
	}
	b := newPost("post-b")
	if err := storage.InsertPost(db, b); err != nil {
		t.Fatalf("InsertPost b: %v", err)
	}

	err := storage.UpdatePost(db, b.ID, b.Version, storage.PostUpdate{
		Title:  b.Title,
		Slug:   "post-a", // collides with a's slug
		Body:   b.Body,
		Status: b.Status,
	})
	if !errors.Is(err, storage.ErrSlugConflict) {
		t.Fatalf("expected ErrSlugConflict, got %v", err)
	}
}

func TestUpdatePostVersionMismatch(t *testing.T) {
	db := newTestDB(t)

	p := newPost("versioned")
	if err := storage.InsertPost(db, p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	err := storage.UpdatePost(db, p.ID, p.Version+1, storage.PostUpdate{
		Title:  "new title",
		Slug:   p.Slug,
		Body:   p.Body,
		Status: p.Status,
	})
	if !errors.Is(err, storage.ErrVersionMismatch) {
		t.Fatalf("expected ErrVersionMismatch, got %v", err)
	}
}

func TestUpdatePostSuccessIncrementsVersion(t *testing.T) {
	db := newTestDB(t)

	p := newPost("update-me")
	if err := storage.InsertPost(db, p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	err := storage.UpdatePost(db, p.ID, p.Version, storage.PostUpdate{
		Title:  "updated title",
		Slug:   p.Slug,
		Body:   p.Body,
		Status: p.Status,
	})
	if err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}

	got, err := storage.GetPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID: %v", err)
	}
	if got.Title != "updated title" {
		t.Fatalf("expected updated title, got %q", got.Title)
	}
	if got.Version != p.Version+1 {
		t.Fatalf("expected version %d, got %d", p.Version+1, got.Version)
	}
}

func TestDeletePostVersionMismatch(t *testing.T) {
	db := newTestDB(t)

	p := newPost("delete-me")
	if err := storage.InsertPost(db, p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	err := storage.DeletePost(db, p.ID, p.Version+1)
	if !errors.Is(err, storage.ErrVersionMismatch) {
		t.Fatalf("expected ErrVersionMismatch, got %v", err)
	}

	// Post must still exist.
	if _, err := storage.GetPostByID(db, p.ID); err != nil {
		t.Fatalf("expected post to still exist, GetPostByID: %v", err)
	}
}

func TestDeletePostSuccess(t *testing.T) {
	db := newTestDB(t)

	p := newPost("delete-me-2")
	if err := storage.InsertPost(db, p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	if err := storage.DeletePost(db, p.ID, p.Version); err != nil {
		t.Fatalf("DeletePost: %v", err)
	}

	if _, err := storage.GetPostByID(db, p.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestListPostsReturnsAllRegardlessOfStatus(t *testing.T) {
	db := newTestDB(t)

	draft := newPost("draft-post")
	draft.Status = "draft"
	if err := storage.InsertPost(db, draft); err != nil {
		t.Fatalf("InsertPost draft: %v", err)
	}

	published := newPost("published-post")
	published.Status = "published"
	published.PublishedAt = sql.NullInt64{Int64: 2000, Valid: true}
	if err := storage.InsertPost(db, published); err != nil {
		t.Fatalf("InsertPost published: %v", err)
	}

	posts, err := storage.ListPosts(db, storage.PostFilter{})
	if err != nil {
		t.Fatalf("ListPosts: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
}

func TestListPostsOrderingAndLimit(t *testing.T) {
	db := newTestDB(t)

	// published_at DESC, id DESC: p2 (published_at=300) should come
	// before p1 (published_at=100).
	p1 := newPost("p1")
	p1.Status = "published"
	p1.PublishedAt = sql.NullInt64{Int64: 100, Valid: true}
	if err := storage.InsertPost(db, p1); err != nil {
		t.Fatalf("InsertPost p1: %v", err)
	}

	p2 := newPost("p2")
	p2.Status = "published"
	p2.PublishedAt = sql.NullInt64{Int64: 300, Valid: true}
	if err := storage.InsertPost(db, p2); err != nil {
		t.Fatalf("InsertPost p2: %v", err)
	}

	posts, err := storage.ListPosts(db, storage.PostFilter{Limit: 1})
	if err != nil {
		t.Fatalf("ListPosts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("expected 1 post due to limit, got %d", len(posts))
	}
	if posts[0].Slug != "p2" {
		t.Fatalf("expected p2 (highest published_at) first, got %s", posts[0].Slug)
	}
}

func TestListPublishedExcludesDrafts(t *testing.T) {
	db := newTestDB(t)

	draft := newPost("draft-only")
	if err := storage.InsertPost(db, draft); err != nil {
		t.Fatalf("InsertPost draft: %v", err)
	}

	published := newPost("published-only")
	published.Status = "published"
	published.PublishedAt = sql.NullInt64{Int64: 500, Valid: true}
	if err := storage.InsertPost(db, published); err != nil {
		t.Fatalf("InsertPost published: %v", err)
	}

	posts, err := storage.ListPublished(db, 10)
	if err != nil {
		t.Fatalf("ListPublished: %v", err)
	}
	if len(posts) != 1 || posts[0].Slug != "published-only" {
		t.Fatalf("expected only the published post, got %+v", posts)
	}
}

func TestGetPublishedBySlugRejectsDraft(t *testing.T) {
	db := newTestDB(t)

	draft := newPost("draft-slug")
	if err := storage.InsertPost(db, draft); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	_, err := storage.GetPublishedBySlug(db, "draft-slug")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for draft slug, got %v", err)
	}
}

func TestGetPublishedBySlugFindsPublished(t *testing.T) {
	db := newTestDB(t)

	published := newPost("published-slug")
	published.Status = "published"
	published.PublishedAt = sql.NullInt64{Int64: 700, Valid: true}
	if err := storage.InsertPost(db, published); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	got, err := storage.GetPublishedBySlug(db, "published-slug")
	if err != nil {
		t.Fatalf("GetPublishedBySlug: %v", err)
	}
	if got.ID != published.ID {
		t.Fatalf("expected id %d, got %d", published.ID, got.ID)
	}
}
