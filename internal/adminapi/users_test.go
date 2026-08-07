package adminapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/auth"
)

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func doUsersRequest(t *testing.T, handler http.Handler, method, path, id string, cookie *http.Cookie, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if id != "" {
		req.SetPathValue("id", id)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCreateUser_ThenListUsers_IncludesNewUser(t *testing.T) {
	db := newTestDB(t)

	rec := doUsersRequest(t, adminapi.CreateUser(db), http.MethodPost, "/api/users", "", nil, map[string]any{
		"username": "editor-1",
		"password": "password123",
		"role":     "editor",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if createdAt, ok := created["created_at"].(float64); !ok || createdAt == 0 {
		t.Fatalf("created_at = %v, want a nonzero timestamp", created["created_at"])
	}

	listRec := doUsersRequest(t, adminapi.ListUsers(db), http.MethodGet, "/api/users", "", nil, nil)
	var users []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(users) != 1 || users[0]["username"] != "editor-1" || users[0]["role"] != "editor" {
		t.Fatalf("users = %v, want one editor-1/editor entry", users)
	}
}

func TestCreateUser_UnknownRole_Returns400(t *testing.T) {
	db := newTestDB(t)

	rec := doUsersRequest(t, adminapi.CreateUser(db), http.MethodPost, "/api/users", "", nil, map[string]any{
		"username": "nobody",
		"password": "password123",
		"role":     "superuser",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateUser_DuplicateUsername_Returns409(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, "dup", "password123", auth.RoleEditor)

	rec := doUsersRequest(t, adminapi.CreateUser(db), http.MethodPost, "/api/users", "", nil, map[string]any{
		"username": "dup",
		"password": "password123",
		"role":     "editor",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

// TestDeleteUser_SelfDeleteAsSoleAdministrator_Returns409 mirrors
// TestUpdateUser_SelfDemoteAsSoleAdministrator_Returns409: ErrSelfTarget is
// checked before the admin count, so this is the reachable 409 for "can't
// remove the last administrator" in a sequential handler test.
func TestDeleteUser_SelfDeleteAsSoleAdministrator_Returns409(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	other := seedUser(t, db, "editor-1", "password123", auth.RoleEditor)

	id := "1"
	path := "/api/users/" + id + "?reassign_to=" + itoa(other)
	rec := doUsersRequest(t, adminapi.DeleteUser(db, db), http.MethodDelete, path, id, cookie, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestDeleteUser_MissingReassignTo_Returns400(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	target := seedUser(t, db, "editor-1", "password123", auth.RoleEditor)

	id := itoa(target)
	rec := doUsersRequest(t, adminapi.DeleteUser(db, db), http.MethodDelete, "/api/users/"+id, id, cookie, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestDeleteUser_SecondAdministrator_Succeeds(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	second := seedUser(t, db, "admin-2", "password123", auth.RoleAdministrator)

	id := itoa(second)
	path := "/api/users/" + id + "?reassign_to=1"
	rec := doUsersRequest(t, adminapi.DeleteUser(db, db), http.MethodDelete, path, id, cookie, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

// TestUpdateUser_SelfDemoteAsSoleAdministrator_Returns409 covers the only
// last-administrator scenario reachable through the handler in a single
// sequential request: CapUsersManage is administrator-only, so any actor
// able to call this endpoint is themselves an administrator, and demoting
// a *different* administrator therefore always leaves the actor behind
// (never triggers ErrLastAdmin — see storage.TestUpdateUserRoleTx for the
// concurrent-demotion race the design guards against). The one 409 an
// end-to-end sequential test can observe is a sole administrator attempting
// to demote themselves, rejected by ErrSelfTarget before the admin count is
// even considered.
func TestUpdateUser_SelfDemoteAsSoleAdministrator_Returns409(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)

	rec := doUsersRequest(t, adminapi.UpdateUser(db, db), http.MethodPatch, "/api/users/1", "1", cookie, map[string]any{"role": "editor"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestUpdateUser_DemoteOtherAdministrator_Succeeds(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	second := seedUser(t, db, "admin-2", "password123", auth.RoleAdministrator)

	id := itoa(second)
	rec := doUsersRequest(t, adminapi.UpdateUser(db, db), http.MethodPatch, "/api/users/"+id, id, cookie, map[string]any{"role": "editor"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestUpdateUser_UnknownRole_Returns400(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	target := seedUser(t, db, "editor-1", "password123", auth.RoleEditor)

	id := itoa(target)
	rec := doUsersRequest(t, adminapi.UpdateUser(db, db), http.MethodPatch, "/api/users/"+id, id, cookie, map[string]any{"role": "superuser"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandler_NonUsersManageRole_Returns403(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, "editor-1", "password123", auth.RoleEditor)
	cookie := loginAs(t, db, "editor-1", "password123")

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	auth.AuthMiddleware(db, adminapi.Handler(db, db)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestDeleteUserMFA_ResetsRegistration(t *testing.T) {
	db := newTestDB(t)
	target := seedUser(t, db, "editor-1", "password123", auth.RoleEditor)

	id := itoa(target)
	rec := doUsersRequest(t, adminapi.DeleteUserMFA(db), http.MethodDelete, "/api/users/"+id+"/mfa", id, nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestDeleteUserMFA_UnknownUser_Returns404(t *testing.T) {
	db := newTestDB(t)

	rec := doUsersRequest(t, adminapi.DeleteUserMFA(db), http.MethodDelete, "/api/users/999/mfa", "999", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// loginAs logs into an already-seeded account, returning the session
// cookie.
func loginAs(t *testing.T, db *sql.DB, username, password string) *http.Cookie {
	t.Helper()
	rec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), username, password)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	return rec.Result().Cookies()[0]
}
