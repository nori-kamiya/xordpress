// Package auth: HTTP middleware for session authentication and CSRF/Origin
// protection (design.md §4).
package auth

import (
	"database/sql"
	"mime"
	"net/http"
)

// AuthMiddleware extracts the session cookie from the request, validates it
// via ValidateSession, and rejects the request with 401 if the cookie is
// missing, unknown, or expired. On success it calls next unchanged.
func AuthMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if _, ok := ValidateSession(r.Context(), db, cookie.Value); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
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
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				http.Error(w, "unsupported content type", http.StatusBadRequest)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
