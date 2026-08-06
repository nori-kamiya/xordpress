package adminapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

const testBaseURL = "https://example.com"

// seedAdmin creates an admin user directly via content.CreateFirstAdmin so
// login tests have known credentials to authenticate against.
func seedAdmin(t *testing.T, db *sql.DB, username, password string) {
	t.Helper()

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	created, err := content.CreateFirstAdmin(t.Context(), db, username, hash, "xordpress")
	if err != nil {
		t.Fatalf("CreateFirstAdmin: %v", err)
	}
	if !created {
		t.Fatalf("CreateFirstAdmin: created = false, want true")
	}
}

func doLoginRequest(t *testing.T, handler http.Handler, username, password string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestLogin_CorrectCredentials_Returns204WithSetCookie(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	handler := adminapi.Login(db, db, testBaseURL)
	rec := doLoginRequest(t, handler, "admin", "password123")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	res := rec.Result()
	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Name != "xordpress_session" {
		t.Errorf("cookie name = %q, want %q", cookies[0].Name, "xordpress_session")
	}
	if cookies[0].Value == "" {
		t.Error("cookie value should not be empty")
	}
}

func TestLogin_WrongPassword_Returns401(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	handler := adminapi.Login(db, db, testBaseURL)
	rec := doLoginRequest(t, handler, "admin", "wrong-password")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("no cookie should be set on failed login")
	}
}

// Regression test for roadmap-to-wp-parity.md P0-3 (brute-force lockout).
func TestLogin_LockedOutAfterRepeatedFailures_Returns429(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	handler := adminapi.Login(db, db, testBaseURL)

	for i := 0; i < storage.MaxLoginFailures; i++ {
		rec := doLoginRequest(t, handler, "admin", "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want %d", i, rec.Code, http.StatusUnauthorized)
		}
	}

	// Even the correct password must now be rejected, distinctly (429, not
	// 401), so the client can tell "locked out" apart from "wrong password".
	rec := doLoginRequest(t, handler, "admin", "password123")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("no cookie should be set while locked out")
	}
}

func TestLogin_UnknownUsername_Returns401(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	handler := adminapi.Login(db, db, testBaseURL)
	rec := doLoginRequest(t, handler, "nobody", "password123")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestLogout_DeletesSessionAndExpiresCookie(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	if loginRec.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d, body=%s", loginRec.Code, http.StatusNoContent, loginRec.Body.String())
	}
	sessionCookie := loginRec.Result().Cookies()[0]

	// The now-deleted session must no longer validate.
	if _, ok := auth.ValidateSession(t.Context(), db, sessionCookie.Value); !ok {
		t.Fatal("precondition failed: session should be valid before logout")
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()

	adminapi.Logout(db).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if _, ok := auth.ValidateSession(t.Context(), db, sessionCookie.Value); ok {
		t.Error("session should be invalid after logout")
	}

	respCookies := rec.Result().Cookies()
	if len(respCookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(respCookies))
	}
	if respCookies[0].Value != "" {
		t.Error("logout cookie value should be cleared")
	}
	if respCookies[0].MaxAge >= 0 {
		t.Errorf("logout cookie MaxAge = %d, want negative (expired)", respCookies[0].MaxAge)
	}
}

func TestLogout_NoCookie_Returns401(t *testing.T) {
	db := newTestDB(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	rec := httptest.NewRecorder()

	adminapi.Logout(db).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}
