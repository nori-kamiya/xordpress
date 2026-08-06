package adminapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
)

// doHandlerRequest sends a request through the assembled Handler mux and
// returns the recorded response.
func doHandlerRequest(t *testing.T, handler http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// assertDispatched fails the test if the response looks like the ServeMux's
// own "no pattern matched" 404, which is distinguishable from any handler in
// this package (all of which reply with a JSON body via writeJSON/writeError)
// by its plain-text content type and fixed body.
func assertDispatched(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "404 page not found") {
		t.Fatalf("request was not dispatched to any registered pattern (got mux's default 404): body=%s", rec.Body.String())
	}
}

func TestHandler_DispatchesEachRegisteredPattern(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Handler(db, db)

	validPost := []byte(`{"title":"Hello","slug":"hello","body":"body text","status":"draft"}`)

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"list posts", http.MethodGet, "/api/posts", nil},
		{"create post", http.MethodPost, "/api/posts", validPost},
		{"get post", http.MethodGet, "/api/posts/1", nil},
		{"update post", http.MethodPut, "/api/posts/1", validPost},
		{"delete post", http.MethodDelete, "/api/posts/1", nil},
		{"delete session", http.MethodDelete, "/api/session", nil},
		{"update settings", http.MethodPut, "/api/settings", []byte(`{"site_name":"My Site"}`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doHandlerRequest(t, handler, tc.method, tc.path, tc.body)
			assertDispatched(t, rec)
		})
	}
}

func TestHandler_UnknownPath_Returns404(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Handler(db, db)

	rec := doHandlerRequest(t, handler, http.MethodGet, "/api/unknown", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "404 page not found") {
		t.Fatalf("body = %q, want mux default 404 body", rec.Body.String())
	}
}

func TestHandler_DoesNotRegisterSetup(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Handler(db, db)

	rec := doHandlerRequest(t, handler, http.MethodPost, "/api/setup", []byte(`{}`))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "404 page not found") {
		t.Fatalf("POST /api/setup: expected Handler to leave this unregistered (mux default 404), got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestHandler_DoesNotRegisterLoginMethod confirms POST /api/session (the
// unauthenticated login endpoint registered separately at the top-level
// mux per design.md §5) is not among the methods this Handler serves for
// /api/session -- only DELETE /api/session (Logout) is registered here, so
// ServeMux's own method-mismatch handling yields 405, not a dispatch to any
// handler in this package.
func TestHandler_DoesNotRegisterLoginMethod(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Handler(db, db)

	rec := doHandlerRequest(t, handler, http.MethodPost, "/api/session", []byte(`{}`))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/session: status = %d, want %d (mux method-not-allowed for the unregistered POST), body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}
