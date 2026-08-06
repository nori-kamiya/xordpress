// Package adminapi: site settings handler (design.md §4).
package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/nori-kamiya/xordpress/internal/content"
)

// settingsRequest is the JSON body of PUT /api/settings.
type settingsRequest struct {
	SiteName string `json:"site_name"`
}

// settingsResponse is the response body of PUT /api/settings (design.md §4).
type settingsResponse struct {
	SiteName string `json:"site_name"`
}

// UpdateSettings returns the handler for PUT /api/settings (design.md §4;
// 要認証 -- enforced by the outer authMiddleware per §5's routing, not by
// this handler itself). It validates site_name (1-100 chars, matching the
// same rule as POST /api/setup) and persists it via content.SetSiteSettings.
func UpdateSettings(contentDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req settingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if len(req.SiteName) < 1 || len(req.SiteName) > 100 {
			writeError(w, http.StatusBadRequest, "invalid site_name")
			return
		}

		if err := content.SetSiteSettings(contentDB, req.SiteName); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, settingsResponse{SiteName: req.SiteName})
	})
}
