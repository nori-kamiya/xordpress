package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthMiddlewareValidSessionPasses(t *testing.T) {
	db := newTestDB(t)
	cookie, err := IssueSession(t.Context(), db, "http://localhost:8080", 1)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	AuthMiddleware(db, okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddlewareMissingCookieUnauthorized(t *testing.T) {
	db := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	rec := httptest.NewRecorder()

	AuthMiddleware(db, okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddlewareExpiredSessionUnauthorized(t *testing.T) {
	db := newTestDB(t)

	const rawToken = "expired-token"
	pastExpiry := time.Now().Add(-1 * time.Hour).Unix()
	if err := storage.InsertSession(db, hashSessionToken(rawToken), 1, pastExpiry); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawToken})
	rec := httptest.NewRecorder()

	AuthMiddleware(db, okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddlewareInvalidCookieUnauthorized(t *testing.T) {
	db := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-real-token"})
	rec := httptest.NewRecorder()

	AuthMiddleware(db, okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCSRFMiddlewareOriginMatchPasses(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/posts", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	CSRFMiddleware("http://localhost:8080", okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestCSRFMiddlewareOriginMissingPasses(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	rec := httptest.NewRecorder()

	CSRFMiddleware("http://localhost:8080", okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestCSRFMiddlewareOriginMismatchForbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/posts", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	CSRFMiddleware("http://localhost:8080", okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestCSRFMiddlewareOriginMismatchForbiddenEvenWithoutAuth verifies that the
// Origin check is evaluated ahead of (i.e. independent of) any downstream
// authentication: even with no way to authenticate, a mismatched Origin
// still yields 403, never a 401 (design.md §4).
func TestCSRFMiddlewareOriginMismatchForbiddenEvenWithoutAuth(t *testing.T) {
	db := newTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/posts", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Content-Type", "application/json")
	// No session cookie is attached: an unauthenticated request.
	rec := httptest.NewRecorder()

	// CSRFMiddleware wraps AuthMiddleware, mirroring the composition order
	// from design.md §5 (Origin/Content-Type check before session auth).
	handler := CSRFMiddleware("http://localhost:8080", AuthMiddleware(db, okHandler()))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (Origin mismatch must win over 401)", rec.Code)
	}
}

func TestCSRFMiddlewareDeleteWithoutContentTypePasses(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	// Deliberately no Content-Type header set.
	rec := httptest.NewRecorder()

	CSRFMiddleware("http://localhost:8080", okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestCSRFMiddlewarePostWithoutContentTypeRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/posts", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	// No Content-Type header set.
	rec := httptest.NewRecorder()

	CSRFMiddleware("http://localhost:8080", okHandler()).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d, want a non-200 rejection", rec.Code)
	}
}

func TestCSRFMiddlewarePutRequiresJSONContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/api/posts/1", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	CSRFMiddleware("http://localhost:8080", okHandler()).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d, want a non-200 rejection for non-JSON Content-Type", rec.Code)
	}
}
