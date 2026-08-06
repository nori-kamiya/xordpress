package adminapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

const testSetupToken = "test-setup-token"

// newTestDB returns a migrated in-memory SQLite DB with no seeded users.
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

	return db
}

func doSetupRequest(t *testing.T, handler http.Handler, token string, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(payload))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func validSetupBody() map[string]string {
	return map[string]string{
		"username":  "admin",
		"password":  "password123",
		"site_name": "xordpress",
	}
}

func TestSetup_WrongToken_Returns401(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Setup(db, db, testSetupToken)

	rec := doSetupRequest(t, handler, "wrong-token", validSetupBody())

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestSetup_MissingToken_Returns401(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Setup(db, db, testSetupToken)

	rec := doSetupRequest(t, handler, "", validSetupBody())

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestSetup_InvalidUsername_Returns400(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Setup(db, db, testSetupToken)

	body := validSetupBody()
	body["username"] = "ab" // too short (min 3)

	rec := doSetupRequest(t, handler, testSetupToken, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestSetup_ShortPassword_Returns400(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Setup(db, db, testSetupToken)

	body := validSetupBody()
	body["password"] = "short1" // 6 chars, below 8-char minimum

	rec := doSetupRequest(t, handler, testSetupToken, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestSetup_InvalidSiteName_Returns400(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Setup(db, db, testSetupToken)

	body := validSetupBody()
	body["site_name"] = ""

	rec := doSetupRequest(t, handler, testSetupToken, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestSetup_ValidRequest_Returns201(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Setup(db, db, testSetupToken)

	rec := doSetupRequest(t, handler, testSetupToken, validSetupBody())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestSetup_SecondSetup_Returns409_EvenWithReusedToken(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.Setup(db, db, testSetupToken)

	first := doSetupRequest(t, handler, testSetupToken, validSetupBody())
	if first.Code != http.StatusCreated {
		t.Fatalf("first setup status = %d, want %d, body=%s", first.Code, http.StatusCreated, first.Body.String())
	}

	second := doSetupRequest(t, handler, testSetupToken, validSetupBody())
	if second.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d, want %d, body=%s", second.Code, http.StatusConflict, second.Body.String())
	}
}
