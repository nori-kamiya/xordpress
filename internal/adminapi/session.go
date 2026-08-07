// Package adminapi: session (login/logout) handlers (design.md §4).
package adminapi

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// sessionCookieName is the name of the session cookie (design.md §4). It
// must match auth.sessionCookieName, which is unexported; kept here so
// Logout can locate and clear the cookie without adding a new export to the
// auth package.
const sessionCookieName = "xordpress_session"

// loginRequest is the JSON body of POST /api/session.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login returns the handler for POST /api/session (design.md §4, §6.1). It
// verifies username/password via auth.Authenticate (mismatch -> 401). If
// the account has a confirmed TOTP registration, it does not issue a
// session yet: it responds 200 {"mfa_required":true,"mfa_token":"..."} and
// the caller must complete POST /api/session/mfa. Otherwise it issues a
// session cookie via auth.IssueSession and responds 204, unchanged from
// before 2FA existed.
//
// baseURL is required to build the session cookie's Secure attribute via
// auth.IssueSession; the work order's Login(content, auth) signature omits
// it (as does design.md §5's routing sketch), but auth.IssueSession has no
// other way to obtain it, so it is added here as a third parameter -- see
// task report for this deviation.
func Login(contentDB *sql.DB, authDB *sql.DB, baseURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		userID, ok, err := auth.Authenticate(r.Context(), contentDB, req.Username, req.Password)
		if errors.Is(err, auth.ErrAccountLocked) {
			writeError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		totpReg, err := storage.GetUserTOTP(contentDB, userID)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err == nil && totpReg.ConfirmedAt != nil {
			locked, err := storage.IsMFALocked(authDB, userID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if locked {
				writeError(w, http.StatusTooManyRequests, "too many failed 2FA attempts, try again later")
				return
			}

			rawToken, err := generateMFAToken()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if err := storage.CreateMFAChallenge(authDB, hashSessionToken(rawToken), userID); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": rawToken})
			return
		}

		cookie, err := auth.IssueSession(r.Context(), authDB, baseURL, userID, r.UserAgent(), clientIP(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
}

// Logout returns the handler for DELETE /api/session (design.md §4; 要認証
// -- in the full routing this endpoint is only reachable through
// authMiddleware, but the handler defends independently so it behaves
// correctly under direct test too). It deletes the session row identified
// by the request's session cookie and clears the cookie client-side.
func Logout(authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if err := storage.DeleteSession(authDB, hashSessionToken(cookie.Value)); err != nil {
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

// hashSessionToken returns the hex-encoded SHA-256 hash of a raw session
// token, matching auth.hashSessionToken's (unexported) algorithm so the
// resulting hash matches the token_hash stored by auth.IssueSession.
func hashSessionToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// mfaTokenBytes is the number of random bytes used for a raw mfa_token
// before base64 encoding, matching auth's session token size.
const mfaTokenBytes = 32

// generateMFAToken returns a fresh cryptographically random mfa_token,
// URL-safe base64 encoded, matching auth.generateSessionToken's scheme (not
// reused directly since that function is unexported).
func generateMFAToken() (string, error) {
	buf := make([]byte, mfaTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// clientIP returns r.RemoteAddr with its port stripped, for storage in
// sessions.created_ip. Falls back to the raw value if it isn't in host:port
// form (e.g. in tests using bare host strings).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
