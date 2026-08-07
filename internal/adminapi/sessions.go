// Package adminapi: session listing/revocation handlers (design.md §5).
package adminapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// sessionResponse is one entry of GET /api/account/sessions' JSON array.
// ID is the session's short id (design.md §5's substr(token_hash,1,16)
// scheme), never the full token_hash. Current is true for the session that
// authenticated the request itself.
type sessionResponse struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	UserAgent string `json:"user_agent"`
	CreatedIP string `json:"created_ip"`
	Current   bool   `json:"current"`
}

// ListSessions returns the handler for GET /api/account/sessions
// (design.md §5). It lists every active session for the requesting user,
// marking the one that authenticated this request as current.
func ListSessions(authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := currentUserID(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var currentID string
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			currentID = hashSessionToken(cookie.Value)[:16]
		}

		sessions, err := storage.ListSessionsByUserID(authDB, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		resp := make([]sessionResponse, len(sessions))
		for i, s := range sessions {
			resp[i] = sessionResponse{
				ID:        s.ID,
				CreatedAt: s.CreatedAt,
				ExpiresAt: s.ExpiresAt,
				UserAgent: s.UserAgent,
				CreatedIP: s.CreatedIP,
				Current:   s.ID == currentID,
			}
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// DeleteOwnSession returns the handler for DELETE /api/account/sessions/{id}
// (design.md §5). id is the short session id returned by ListSessions. A
// session that no longer exists is treated as already revoked (204,
// idempotent — tasks.md T8's "対象なしは204" clause). A session that exists
// but belongs to another user responds 404 (design.md §5's "他人のセッシ
// ョンは404" clause), matching the "not found" the caller would see for a
// garbage id -- distinguishing it as 403 would leak that the id is valid
// for someone else.
func DeleteOwnSession(authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := currentUserID(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		id := r.PathValue("id")
		ownerID, err := storage.SessionOwnerByPrefix(authDB, id)
		if errors.Is(err, storage.ErrNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if ownerID != userID {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		if err := storage.DeleteSessionByPrefix(authDB, userID, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// DeleteUserSessions returns the handler for DELETE /api/users/{id}/sessions
// (design.md §5, CapUsersManage). It force-revokes every session belonging
// to the target user, reusing the same storage call ChangePassword uses for
// self-revocation on password rotation. The target user's existence is
// checked against contentDB (where users live) and the revocation runs
// against authDB (where sessions live), matching the split ChangePassword
// and posts.go's currentPrincipal already establish.
func DeleteUserSessions(contentDB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		if _, err := storage.GetUserByID(contentDB, id); errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if err := storage.DeleteSessionsByUserID(authDB, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
