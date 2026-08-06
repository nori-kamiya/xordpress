package adminui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_ServesIndexAtAdminRoot(t *testing.T) {
	h := Handler()

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected Content-Type text/html, got %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("expected non-empty body")
	}
}

func TestHandler_FallsBackToIndexForUnknownSubpath(t *testing.T) {
	h := Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rootRec := httptest.NewRecorder()
	h.ServeHTTP(rootRec, rootReq)

	req := httptest.NewRequest(http.MethodGet, "/admin/some/unknown/route", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (SPA fallback), got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected Content-Type text/html, got %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("expected non-empty body")
	}
	if rec.Body.String() != rootRec.Body.String() {
		t.Fatalf("expected fallback content to match /admin/ index content")
	}
}

func TestHandler_RedirectsAdminWithoutTrailingSlash(t *testing.T) {
	h := Handler()

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected status 301, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/" {
		t.Fatalf("expected redirect to /admin/, got %q", loc)
	}
}
