package adminapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/content"
)

func doSettingsRequest(t *testing.T, handler http.Handler, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestUpdateSettings_ValidSiteName_Returns200AndPersists(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.UpdateSettings(db)

	rec := doSettingsRequest(t, handler, map[string]string{"site_name": "My Site"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		SiteName string `json:"site_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.SiteName != "My Site" {
		t.Fatalf("response site_name = %q, want %q", resp.SiteName, "My Site")
	}

	settings, err := content.GetSiteSettings(db)
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if settings.Name != "My Site" {
		t.Fatalf("persisted site_name = %q, want %q", settings.Name, "My Site")
	}
}

func TestUpdateSettings_EmptySiteName_Returns400(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.UpdateSettings(db)

	rec := doSettingsRequest(t, handler, map[string]string{"site_name": ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUpdateSettings_TooLongSiteName_Returns400(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.UpdateSettings(db)

	rec := doSettingsRequest(t, handler, map[string]string{"site_name": strings.Repeat("a", 101)})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUpdateSettings_MalformedBody_Returns400(t *testing.T) {
	db := newTestDB(t)
	handler := adminapi.UpdateSettings(db)

	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
