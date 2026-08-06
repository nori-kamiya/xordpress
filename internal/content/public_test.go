package content_test

import (
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// TestListPublishedExcludesDrafts is a regression test for the "public
// page leaks drafts" bug: with one draft and one published post present,
// ListPublished must return only the published one.
func TestListPublishedExcludesDrafts(t *testing.T) {
	db := newTestDB(t)

	if _, err := content.Create(db, content.CreateInput{Slug: "draft-post", Title: "D", Body: "<p>d</p>", Status: "draft", AuthorID: 1}); err != nil {
		t.Fatalf("Create draft: %v", err)
	}
	published, err := content.Create(db, content.CreateInput{Slug: "published-post", Title: "P", Body: "<p>p</p>", Status: "published", AuthorID: 1})
	if err != nil {
		t.Fatalf("Create published: %v", err)
	}

	posts, err := content.ListPublished(db)
	if err != nil {
		t.Fatalf("ListPublished: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("expected 1 published post, got %d", len(posts))
	}
	if posts[0].Slug != published.Slug {
		t.Fatalf("expected slug %q, got %q", published.Slug, posts[0].Slug)
	}
	if posts[0].Status != "published" {
		t.Fatalf("expected status published, got %q", posts[0].Status)
	}
}

// TestGetPublishedExcludesDrafts is a regression test for the same bug on
// the single-post lookup path: GetPublished must return ErrNotFound for a
// draft slug, and the actual post for a published slug.
func TestGetPublishedExcludesDrafts(t *testing.T) {
	db := newTestDB(t)

	if _, err := content.Create(db, content.CreateInput{Slug: "draft-post", Title: "D", Body: "<p>d</p>", Status: "draft", AuthorID: 1}); err != nil {
		t.Fatalf("Create draft: %v", err)
	}
	if _, err := content.Create(db, content.CreateInput{Slug: "published-post", Title: "P", Body: "<p>p</p>", Status: "published", AuthorID: 1}); err != nil {
		t.Fatalf("Create published: %v", err)
	}

	_, err := content.GetPublished(db, "draft-post")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for draft slug, got %v", err)
	}

	got, err := content.GetPublished(db, "published-post")
	if err != nil {
		t.Fatalf("GetPublished: %v", err)
	}
	if got.Slug != "published-post" {
		t.Fatalf("expected slug published-post, got %q", got.Slug)
	}
}
