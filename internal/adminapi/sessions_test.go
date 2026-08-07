package adminapi_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// sessionIDForToken mirrors design.md §5's id scheme (the first 16 hex
// characters of sha256(rawToken)) so tests can assert which listed session
// corresponds to a given cookie, independent of the handler under test.
func sessionIDForToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])[:16]
}

// seedUser creates a user with the given role directly via CreateUserTx, so
// tests can set up a second account without going through the setup/signup
// flow (which only ever creates administrators).
func seedUser(t *testing.T, db *sql.DB, username, password string, role auth.Role) int64 {
	t.Helper()

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := storage.CreateUserTx(tx, username, hash, string(role))
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

func doListSessionsRequest(t *testing.T, handler http.Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/account/sessions", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func doDeleteOwnSessionRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, id string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/account/sessions/"+id, nil)
	req.SetPathValue("id", id)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func doDeleteUserSessionsRequest(t *testing.T, handler http.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+id+"/sessions", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestListSessions_ReturnsOwnSessionsWithCurrentFlagged(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	login1 := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	cookie1 := login1.Result().Cookies()[0]
	login2 := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	cookie2 := login2.Result().Cookies()[0]

	rec := doListSessionsRequest(t, adminapi.ListSessions(db), cookie2)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var sessions []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}

	wantCurrentID := sessionIDForToken(cookie2.Value)
	wantOtherID := sessionIDForToken(cookie1.Value)

	byID := make(map[string]bool)
	for _, s := range sessions {
		id, ok := s["id"].(string)
		if !ok {
			t.Fatalf("session entry missing string id: %v", s)
		}
		byID[id] = s["current"] == true
	}

	if current, ok := byID[wantCurrentID]; !ok || !current {
		t.Errorf("session for the request's own cookie (id=%s) should have current=true, got present=%v current=%v", wantCurrentID, ok, current)
	}
	if current, ok := byID[wantOtherID]; !ok || current {
		t.Errorf("session for the other cookie (id=%s) should have current=false, got present=%v current=%v", wantOtherID, ok, current)
	}
}

func TestListSessions_NoCookie_Returns401(t *testing.T) {
	db := newTestDB(t)

	rec := doListSessionsRequest(t, adminapi.ListSessions(db), nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestDeleteOwnSession_OtherSession_RevokesItButNotCaller(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")

	login1 := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	cookie1 := login1.Result().Cookies()[0]
	login2 := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	cookie2 := login2.Result().Cookies()[0]

	listRec := doListSessionsRequest(t, adminapi.ListSessions(db), cookie1)
	var sessions []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var otherID string
	for _, s := range sessions {
		if s["current"] != true {
			otherID = s["id"].(string)
		}
	}
	if otherID == "" {
		t.Fatal("could not find the non-current session id")
	}

	rec := doDeleteOwnSessionRequest(t, adminapi.DeleteOwnSession(db), cookie1, otherID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if _, ok := auth.ValidateSession(t.Context(), db, cookie2.Value); ok {
		t.Error("the revoked session should no longer validate")
	}
	if _, ok := auth.ValidateSession(t.Context(), db, cookie1.Value); !ok {
		t.Error("the caller's own session should remain valid")
	}
}

func TestDeleteOwnSession_UnknownID_Returns204Idempotent(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	login := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	cookie := login.Result().Cookies()[0]

	rec := doDeleteOwnSessionRequest(t, adminapi.DeleteOwnSession(db), cookie, "0000000000000000")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestDeleteOwnSession_AnotherUsersSessionID_Returns404(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	seedUser(t, db, "editor", "password123", auth.RoleEditor)

	adminLogin := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	adminCookie := adminLogin.Result().Cookies()[0]
	editorLogin := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "editor", "password123")
	editorCookie := editorLogin.Result().Cookies()[0]

	editorSessions := doListSessionsRequest(t, adminapi.ListSessions(db), editorCookie)
	var sessions []map[string]any
	if err := json.Unmarshal(editorSessions.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	editorSessionID := sessions[0]["id"].(string)

	rec := doDeleteOwnSessionRequest(t, adminapi.DeleteOwnSession(db), adminCookie, editorSessionID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	if _, ok := auth.ValidateSession(t.Context(), db, editorCookie.Value); !ok {
		t.Error("the other user's session must remain valid after a 404")
	}
}

func TestDeleteOwnSession_NoCookie_Returns401(t *testing.T) {
	db := newTestDB(t)

	rec := doDeleteOwnSessionRequest(t, adminapi.DeleteOwnSession(db), nil, "0000000000000000")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestDeleteUserSessions_RevokesAllSessionsForTargetUser(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	editorID := seedUser(t, db, "editor", "password123", auth.RoleEditor)

	login1 := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "editor", "password123")
	cookie1 := login1.Result().Cookies()[0]
	login2 := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "editor", "password123")
	cookie2 := login2.Result().Cookies()[0]

	rec := doDeleteUserSessionsRequest(t, adminapi.DeleteUserSessions(db, db), strconv.FormatInt(editorID, 10))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if _, ok := auth.ValidateSession(t.Context(), db, cookie1.Value); ok {
		t.Error("session 1 should be revoked")
	}
	if _, ok := auth.ValidateSession(t.Context(), db, cookie2.Value); ok {
		t.Error("session 2 should be revoked")
	}
}

func TestDeleteUserSessions_UnknownUserID_Returns404(t *testing.T) {
	db := newTestDB(t)

	rec := doDeleteUserSessionsRequest(t, adminapi.DeleteUserSessions(db, db), "999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestDeleteUserSessions_InvalidID_Returns400(t *testing.T) {
	db := newTestDB(t)

	rec := doDeleteUserSessionsRequest(t, adminapi.DeleteUserSessions(db, db), "not-a-number")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
