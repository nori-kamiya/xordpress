// Package adminapi: account (password change) handler.
//
// roadmap-to-wp-parity.md P0-2: before this, there was no way to change or
// recover the single admin's password once initial setup completed --
// forgetting it meant the instance was permanently locked out.
package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// changePasswordRequest is the JSON body of PUT /api/account/password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// accountResponse is the body of GET /api/account.
type accountResponse struct {
	ID           int64    `json:"id"`
	Username     string   `json:"username"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
}

// GetAccount returns the handler for GET /api/account (design.md §5): the
// SPA's source of truth for "who am I and what can I do", used to decide
// which navigation/actions to show (cosmetic only — every endpoint still
// enforces its own capability server-side).
func GetAccount(contentDB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := currentPrincipal(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		user, err := storage.GetUserByID(contentDB, p.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, accountResponse{
			ID:           user.ID,
			Username:     user.Username,
			Role:         string(p.Role),
			Capabilities: p.Role.CapabilityNames(),
		})
	})
}

// ChangePassword returns the handler for PUT /api/account/password (要認証
// -- enforced by the outer authMiddleware per app.go's routing, not by this
// handler itself, matching UpdateSettings' pattern). It verifies
// current_password against the requesting user's stored hash, and on match
// replaces it with new_password's hash. Every session for that user --
// including the one that made this request -- is then invalidated and the
// request's own session cookie is cleared client-side, the same as Logout,
// so a stolen/leaked old session token stops working the moment the
// password is rotated. The caller must log in again afterward.
func ChangePassword(contentDB *sql.DB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := currentUserID(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if len(req.NewPassword) < 8 {
			writeError(w, http.StatusBadRequest, "invalid new_password")
			return
		}

		user, err := storage.GetUserByID(contentDB, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !auth.VerifyPassword(user.PasswordHash, req.CurrentPassword) {
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}

		newHash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := storage.UpdatePasswordHash(contentDB, userID, newHash); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := storage.DeleteSessionsByUserID(authDB, userID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})
}
