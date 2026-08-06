package adminapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/auth"
)

func doChangePasswordRequest(
	t *testing.T, handler http.Handler, cookie *http.Cookie, currentPassword, newPassword string,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{"current_password": currentPassword, "new_password": newPassword})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/account/password", bytes.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestChangePassword_CorrectCurrentPassword_RotatesHashAndInvalidatesAllSessions(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	sessionCookie := loginRec.Result().Cookies()[0]

	rec := doChangePasswordRequest(t, adminapi.ChangePassword(db, db), sessionCookie, "password123", "new-password-456")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// The session used to make the change must be invalidated too (not just
	// other sessions), so a leaked old token can't outlive a rotation.
	if _, ok := auth.ValidateSession(t.Context(), db, sessionCookie.Value); ok {
		t.Error("session should be invalidated after a password change")
	}

	respCookies := rec.Result().Cookies()
	if len(respCookies) != 1 || respCookies[0].Value != "" {
		t.Error("response should clear the session cookie, like Logout does")
	}

	// Old password must no longer authenticate; new password must.
	if oldRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123"); oldRec.Code != http.StatusUnauthorized {
		t.Errorf("login with old password status = %d, want 401", oldRec.Code)
	}
	if newRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "new-password-456"); newRec.Code != http.StatusNoContent {
		t.Errorf("login with new password status = %d, want 204", newRec.Code)
	}
}

func TestChangePassword_WrongCurrentPassword_Returns401AndKeepsSessionValid(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	sessionCookie := loginRec.Result().Cookies()[0]

	rec := doChangePasswordRequest(t, adminapi.ChangePassword(db, db), sessionCookie, "wrong-password", "new-password-456")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	// A wrong *current* password must not tear down the still-valid session.
	if _, ok := auth.ValidateSession(t.Context(), db, sessionCookie.Value); !ok {
		t.Error("session should remain valid after a rejected password-change attempt")
	}
}

func TestChangePassword_NewPasswordTooShort_Returns400(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	sessionCookie := loginRec.Result().Cookies()[0]

	rec := doChangePasswordRequest(t, adminapi.ChangePassword(db, db), sessionCookie, "password123", "short")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestChangePassword_NoCookie_Returns401(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	rec := doChangePasswordRequest(t, adminapi.ChangePassword(db, db), nil, "password123", "new-password-456")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}
