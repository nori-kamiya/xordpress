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
)

// newPostsMux registers the full set of posts patterns on a local
// *http.ServeMux so r.PathValue("id") resolves the same way it will once
// T19 wires the real adminapi.Handler.
func newPostsMux(db *sql.DB) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /api/posts", adminapi.ListPosts(db))
	mux.Handle("POST /api/posts", adminapi.CreatePost(db, db))
	mux.Handle("GET /api/posts/{id}", adminapi.GetPost(db))
	mux.Handle("PUT /api/posts/{id}", adminapi.UpdatePost(db))
	mux.Handle("DELETE /api/posts/{id}", adminapi.DeletePost(db))
	return mux
}

// loginCookie seeds an admin user and logs in via adminapi.Login, returning
// the resulting session cookie so CreatePost can resolve author_id.
func loginCookie(t *testing.T, db *sql.DB) *http.Cookie {
	t.Helper()

	seedAdmin(t, db, "admin", "password123")
	rec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	return rec.Result().Cookies()[0]
}

func doPostRequest(t *testing.T, mux *http.ServeMux, method, path string, cookie *http.Cookie, ifMatch string, body map[string]string) *httptest.ResponseRecorder {
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
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodePostResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response %s: %v", rec.Body.String(), err)
	}
	return out
}

// TestPostsLifecycle_CreateListGetUpdateDelete exercises the full
// create->list->get->update(correct/wrong If-Match)->delete round trip
// (T18 verify).
func TestPostsLifecycle_CreateListGetUpdateDelete(t *testing.T) {
	db := newTestDB(t)
	mux := newPostsMux(db)
	cookie := loginCookie(t, db)

	// Create.
	createRec := doPostRequest(t, mux, http.MethodPost, "/api/posts", cookie, "", map[string]string{
		"title": "Hello World", "slug": "hello-world", "body": "<p>hi</p>", "status": "draft",
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	created := decodePostResponse(t, createRec)
	if v, _ := created["version"].(float64); v != 1 {
		t.Errorf("create version = %v, want 1", created["version"])
	}
	postID := created["id"].(float64)

	// List: envelope shape + version included.
	listRec := doPostRequest(t, mux, http.MethodGet, "/api/posts", cookie, "", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listBody struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listBody.Total != 1 {
		t.Fatalf("list total = %d, want 1", listBody.Total)
	}
	if len(listBody.Items) != 1 {
		t.Fatalf("list items = %d, want 1", len(listBody.Items))
	}
	if _, ok := listBody.Items[0]["version"]; !ok {
		t.Error("list item missing version field")
	}

	// Get.
	getPath := "/api/posts/" + strconv.FormatInt(int64(postID), 10)
	getRec := doPostRequest(t, mux, http.MethodGet, getPath, cookie, "", nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d, body=%s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	gotten := decodePostResponse(t, getRec)
	if gotten["slug"] != "hello-world" {
		t.Errorf("get slug = %v, want hello-world", gotten["slug"])
	}

	// Update with wrong If-Match -> 412.
	wrongUpdateRec := doPostRequest(t, mux, http.MethodPut, getPath, cookie, `"99"`, map[string]string{
		"title": "Updated", "slug": "hello-world", "body": "<p>updated</p>", "status": "draft",
	})
	if wrongUpdateRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("update wrong If-Match status = %d, want %d, body=%s", wrongUpdateRec.Code, http.StatusPreconditionFailed, wrongUpdateRec.Body.String())
	}

	// Update with correct If-Match -> 200.
	updateRec := doPostRequest(t, mux, http.MethodPut, getPath, cookie, `"1"`, map[string]string{
		"title": "Updated", "slug": "hello-world", "body": "<p>updated</p>", "status": "published",
	})
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body=%s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}
	updated := decodePostResponse(t, updateRec)
	if v, _ := updated["version"].(float64); v != 2 {
		t.Errorf("update version = %v, want 2", updated["version"])
	}

	// Delete with wrong If-Match -> 412.
	wrongDeleteRec := doPostRequest(t, mux, http.MethodDelete, getPath, cookie, `"1"`, nil)
	if wrongDeleteRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("delete wrong If-Match status = %d, want %d, body=%s", wrongDeleteRec.Code, http.StatusPreconditionFailed, wrongDeleteRec.Body.String())
	}

	// Delete with correct If-Match -> 204.
	deleteRec := doPostRequest(t, mux, http.MethodDelete, getPath, cookie, `"2"`, nil)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body=%s", deleteRec.Code, http.StatusNoContent, deleteRec.Body.String())
	}

	// Get after delete -> 404.
	afterDeleteRec := doPostRequest(t, mux, http.MethodGet, getPath, cookie, "", nil)
	if afterDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d, body=%s", afterDeleteRec.Code, http.StatusNotFound, afterDeleteRec.Body.String())
	}
}

func TestCreatePost_ReservedSlug_Returns400(t *testing.T) {
	db := newTestDB(t)
	mux := newPostsMux(db)
	cookie := loginCookie(t, db)

	rec := doPostRequest(t, mux, http.MethodPost, "/api/posts", cookie, "", map[string]string{
		"title": "Reserved", "slug": "admin", "body": "<p>x</p>", "status": "draft",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreatePost_SlugConflict_Returns409(t *testing.T) {
	db := newTestDB(t)
	mux := newPostsMux(db)
	cookie := loginCookie(t, db)

	first := doPostRequest(t, mux, http.MethodPost, "/api/posts", cookie, "", map[string]string{
		"title": "First", "slug": "dup-slug", "body": "<p>a</p>", "status": "draft",
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d, body=%s", first.Code, http.StatusCreated, first.Body.String())
	}

	second := doPostRequest(t, mux, http.MethodPost, "/api/posts", cookie, "", map[string]string{
		"title": "Second", "slug": "dup-slug", "body": "<p>b</p>", "status": "draft",
	})
	if second.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want %d, body=%s", second.Code, http.StatusConflict, second.Body.String())
	}
}

func TestGetPost_NonNumericID_Returns400(t *testing.T) {
	db := newTestDB(t)
	mux := newPostsMux(db)
	cookie := loginCookie(t, db)

	rec := doPostRequest(t, mux, http.MethodGet, "/api/posts/not-a-number", cookie, "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
