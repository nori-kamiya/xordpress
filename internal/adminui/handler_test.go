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
	if !strings.Contains(rec.Body.String(), "Admin UI") {
		t.Fatalf("expected body to contain placeholder index.html content, got %q", rec.Body.String())
	}
}

func TestHandler_FallsBackToIndexForUnknownSubpath(t *testing.T) {
	h := Handler()

	req := httptest.NewRequest(http.MethodGet, "/admin/some/unknown/route", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (SPA fallback), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Admin UI") {
		t.Fatalf("expected fallback body to contain index.html content, got %q", rec.Body.String())
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
