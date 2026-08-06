// Package adminapi: HTTP handlers for the authenticated admin API
// (design.md §4).
package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/content"
)

// usernamePattern matches the setup/login username rule (design.md §4):
// 3-32 characters, alphanumeric plus hyphen and underscore.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// setupRequest is the JSON body of POST /api/setup.
type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	SiteName string `json:"site_name"`
}

// Setup returns the handler for POST /api/setup (design.md §4). It verifies
// the Authorization: Bearer header against setupToken via
// auth.CompareSetupToken (mismatch -> 401), validates the request body,
// hashes the password via auth.HashPassword, and calls
// content.CreateFirstAdmin to atomically create the first admin user. A
// created=false result (an admin already exists) maps to 409; success maps
// to 201.
func Setup(contentDB *sql.DB, authDB *sql.DB, setupToken string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !auth.CompareSetupToken(setupToken, provided) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req setupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if !usernamePattern.MatchString(req.Username) {
			writeError(w, http.StatusBadRequest, "invalid username")
			return
		}
		if len(req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "invalid password")
			return
		}
		if len(req.SiteName) < 1 || len(req.SiteName) > 100 {
			writeError(w, http.StatusBadRequest, "invalid site_name")
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		created, err := content.CreateFirstAdmin(r.Context(), contentDB, req.Username, hash, req.SiteName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !created {
			writeError(w, http.StatusConflict, "setup already completed")
			return
		}

		w.WriteHeader(http.StatusCreated)
	})
}

// writeError writes a unified {"error": "message"} JSON response
// (design.md §4).
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
