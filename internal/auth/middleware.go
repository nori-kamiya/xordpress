// Package auth: HTTP middleware for session authentication and CSRF/Origin
// protection (design.md §4).
package auth

import (
	"database/sql"
	"encoding/json"
	"mime"
	"net/http"
)

// writeError writes a JSON {"error": "..."} body, matching adminapi's
// writeError shape. roadmap-to-wp-parity.md P0-4: this middleware used to
// reject requests with http.Error's plain-text body while every handler
// inside adminapi returned {"error":"..."} for the same status codes --  an
// inconsistency clients had to special-case. All rejections from this
// middleware now use the same JSON shape as the handlers it wraps.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// AuthMiddleware extracts the session cookie from the request, validates it
// via ValidateSession, and rejects the request with 401 if the cookie is
// missing, unknown, or expired. On success it calls next unchanged.
func AuthMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if _, ok := ValidateSession(r.Context(), db, cookie.Value); !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware rejects requests whose Origin header is present and does
// not match baseURL with 403, regardless of authentication state (design.md
// §4: Origin mismatch is evaluated before session authentication so the
// response does not leak whether the request would otherwise have been
// authenticated). A missing Origin header is allowed.
//
// For POST and PUT requests (methods that carry a body), it additionally
// requires a Content-Type of application/json; DELETE is exempt since it
// carries no body.
func CSRFMiddleware(baseURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin != baseURL {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				writeError(w, http.StatusBadRequest, "unsupported content type")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
