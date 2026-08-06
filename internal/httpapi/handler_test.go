package httpapi_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/httpapi"
	"github.com/nori-kamiya/xordpress/internal/storage"
	"github.com/nori-kamiya/xordpress/internal/theme"
)

// newTestDB returns a migrated in-memory SQLite DB with one user (id=1) to
// satisfy posts.author_id's foreign key, mirroring internal/content's
// helper of the same name.
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

func doRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHandler_IndexRendersPublishedPosts(t *testing.T) {
	db := newTestDB(t)
	if err := content.SetSiteSettings(db, "My Site"); err != nil {
		t.Fatalf("SetSiteSettings: %v", err)
	}
	if _, err := content.Create(db, content.CreateInput{Slug: "hello", Title: "Hello World", Body: "<p>hi</p>", Status: "published", AuthorID: 1}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	handler := httpapi.Handler(db, theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "My Site") {
		t.Errorf("expected site name in body, got: %s", body)
	}
	if !strings.Contains(body, "Hello World") {
		t.Errorf("expected post title in body, got: %s", body)
	}
}

func TestHandler_SlugRendersPublishedPost(t *testing.T) {
	db := newTestDB(t)
	if err := content.SetSiteSettings(db, "My Site"); err != nil {
		t.Fatalf("SetSiteSettings: %v", err)
	}
	if _, err := content.Create(db, content.CreateInput{Slug: "hello", Title: "Hello World", Body: "<p>hi there</p>", Status: "published", AuthorID: 1}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	handler := httpapi.Handler(db, theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodGet, "/hello")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Hello World") {
		t.Errorf("expected post title in body, got: %s", body)
	}
	if !strings.Contains(body, "hi there") {
		t.Errorf("expected post body in body, got: %s", body)
	}
	if !strings.Contains(body, "My Site") {
		t.Errorf("expected site name in body, got: %s", body)
	}
}

// TestHandler_DraftSlugReturns404 is a regression test: a draft post's slug
// must never be reachable through the public handler (design.md §3's
// public/admin read split).
func TestHandler_DraftSlugReturns404(t *testing.T) {
	db := newTestDB(t)
	if _, err := content.Create(db, content.CreateInput{Slug: "draft-post", Title: "Draft", Body: "<p>draft</p>", Status: "draft", AuthorID: 1}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	handler := httpapi.Handler(db, theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodGet, "/draft-post")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a draft slug, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_UnknownSlugReturns404(t *testing.T) {
	db := newTestDB(t)

	handler := httpapi.Handler(db, theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodGet, "/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a nonexistent slug, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_NonGetHeadMethodReturns405(t *testing.T) {
	db := newTestDB(t)

	handler := httpapi.Handler(db, theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodPost, "/")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_HeadIndexReturns200(t *testing.T) {
	db := newTestDB(t)
	if err := content.SetSiteSettings(db, "My Site"); err != nil {
		t.Fatalf("SetSiteSettings: %v", err)
	}

	handler := httpapi.Handler(db, theme.New(t.TempDir()))
	rec := doRequest(handler, http.MethodHead, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD /, got %d", rec.Code)
	}
}
