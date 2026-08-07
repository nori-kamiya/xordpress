// Package adminapi: user management handlers (design.md §5, CapUsersManage).
package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// userResponse is one entry of GET /api/users' JSON array and the body of
// GET/POST/PATCH /api/users/{id}.
type userResponse struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
}

func toUserResponse(u *storage.AdminUser) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt}
}

// ListUsers returns the handler for GET /api/users (design.md §5).
func ListUsers(contentDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		users, err := storage.ListUsers(contentDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		resp := make([]userResponse, len(users))
		for i, u := range users {
			resp[i] = toUserResponse(u)
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// createUserRequest is the JSON body of POST /api/users.
type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUser returns the handler for POST /api/users (design.md §5).
// Rejects an unknown role or an out-of-policy username/password with 400,
// and an already-taken username with 409.
func CreateUser(contentDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
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
		if !auth.IsValidRole(auth.Role(req.Role)) {
			writeError(w, http.StatusBadRequest, "invalid role")
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		tx, err := contentDB.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer tx.Rollback()

		id, err := storage.CreateUserTx(tx, req.Username, hash, req.Role)
		if errors.Is(err, storage.ErrUsernameTaken) {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		u, err := storage.GetUserByIDTx(tx, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusCreated, toUserResponse(u))
	})
}

// parseUserID extracts and validates the {id} path value shared by every
// /api/users/{id}... route, writing a 400 response and returning ok=false
// on failure.
func parseUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// GetUser returns the handler for GET /api/users/{id} (design.md §5).
func GetUser(contentDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUserID(w, r)
		if !ok {
			return
		}

		tx, err := contentDB.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer tx.Rollback()

		u, err := storage.GetUserByIDTx(tx, id)
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, toUserResponse(u))
	})
}

// updateUserRequest is the JSON body of PATCH /api/users/{id}. Password is
// optional: when set, an administrator-initiated reset also revokes every
// existing session for the target user (mirroring ChangePassword's
// self-service behavior).
type updateUserRequest struct {
	Role     string  `json:"role"`
	Password *string `json:"password"`
}

// UpdateUser returns the handler for PATCH /api/users/{id} (design.md §5).
// Role changes and the last-administrator check run in a single
// transaction (design.md §6.2) to prevent two concurrent demotions from
// both observing "another administrator exists".
func UpdateUser(contentDB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUserID(w, r)
		if !ok {
			return
		}

		actorID, ok := currentUserID(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req updateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if !auth.IsValidRole(auth.Role(req.Role)) {
			writeError(w, http.StatusBadRequest, "invalid role")
			return
		}
		if req.Password != nil && len(*req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "invalid password")
			return
		}

		var newHash string
		if req.Password != nil {
			hash, err := auth.HashPassword(*req.Password)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			newHash = hash
		}

		tx, err := contentDB.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer tx.Rollback()

		if err := storage.UpdateUserRoleTx(tx, id, req.Role, actorID); err != nil {
			switch {
			case errors.Is(err, storage.ErrSelfTarget):
				writeError(w, http.StatusConflict, "cannot change your own role")
			case errors.Is(err, storage.ErrNotFound):
				writeError(w, http.StatusNotFound, "not found")
			case errors.Is(err, storage.ErrLastAdmin):
				writeError(w, http.StatusConflict, "cannot remove the last administrator")
			default:
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		u, err := storage.GetUserByIDTx(tx, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// Password reset (if requested) and the resulting session revocation
		// run after the role-change transaction commits: they don't share
		// the last-administrator invariant that requires the role check and
		// its update to be atomic, and ChangePassword follows the same
		// sequential (non-tx) pattern for the equivalent self-service flow.
		if req.Password != nil {
			if err := storage.UpdatePasswordHash(contentDB, id, newHash); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if err := storage.DeleteSessionsByUserID(authDB, id); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}

		writeJSON(w, http.StatusOK, toUserResponse(u))
	})
}

// DeleteUser returns the handler for DELETE /api/users/{id} (design.md §5,
// §6.2). ?reassign_to=<id> is required: missing, self-referential, or
// naming a nonexistent user all respond 400 before any mutation runs.
func DeleteUser(contentDB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUserID(w, r)
		if !ok {
			return
		}

		actorID, ok := currentUserID(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		reassignTo, err := strconv.ParseInt(r.URL.Query().Get("reassign_to"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "reassign_to is required")
			return
		}

		tx, err := contentDB.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer tx.Rollback()

		if err := storage.DeleteUserCascadeTx(tx, id, reassignTo, actorID); err != nil {
			switch {
			case errors.Is(err, storage.ErrSelfTarget):
				writeError(w, http.StatusConflict, "cannot delete your own account")
			case errors.Is(err, storage.ErrSelfReassign):
				writeError(w, http.StatusBadRequest, "reassign_to cannot equal the deleted user")
			case errors.Is(err, storage.ErrNotFound):
				writeError(w, http.StatusNotFound, "not found")
			case errors.Is(err, storage.ErrReassignTargetNotFound):
				writeError(w, http.StatusBadRequest, "reassign_to user not found")
			case errors.Is(err, storage.ErrLastAdmin):
				writeError(w, http.StatusConflict, "cannot delete the last administrator")
			default:
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// DeleteUserMFA returns the handler for DELETE /api/users/{id}/mfa
// (design.md §5): a lockout-recovery reset that clears the target user's
// TOTP registration, recovery codes, and any pending challenge, without
// touching the account itself.
func DeleteUserMFA(contentDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUserID(w, r)
		if !ok {
			return
		}

		tx, err := contentDB.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer tx.Rollback()

		if _, err := storage.GetUserByIDTx(tx, id); errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if err := storage.ResetUserMFATx(tx, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
