// Package auth: session issuance and validation.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// sessionCookieName is the name of the session cookie (design.md §4).
const sessionCookieName = "xordpress_session"

// sessionTTL is the fixed session lifetime: 7 days, no sliding extension
// (design.md §4, §7).
const sessionTTL = 7 * 24 * time.Hour

// sessionTokenBytes is the number of random bytes used for the raw session
// token before base64 encoding.
const sessionTokenBytes = 32

// IssueSession generates a new random session token, stores its SHA-256
// hash in storage.sessions (the raw token is never persisted), and returns
// the *http.Cookie to set on the response.
//
// The cookie is built per design.md §4: name xordpress_session, Path=/,
// HttpOnly, SameSite=Lax, expiring 7 days from now. The Secure attribute is
// determined by baseURL's scheme (not the live connection's TLS state), so
// it is correctly set even when XordPress runs behind a TLS-terminating
// reverse proxy.
func IssueSession(ctx context.Context, db *sql.DB, baseURL string, userID int64) (*http.Cookie, error) {
	rawToken, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("auth: generate session token: %w", err)
	}

	expiresAt := time.Now().Add(sessionTTL)

	if err := storage.InsertSession(db, hashSessionToken(rawToken), userID, expiresAt.Unix()); err != nil {
		return nil, fmt.Errorf("auth: issue session: %w", err)
	}

	secure := baseURLIsSecure(baseURL)

	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

// ValidateSession reports whether token corresponds to a non-expired
// session, returning the associated user ID if so.
//
// Expiry is checked lazily at validation time (design.md §7): a session row
// past its expires_at is treated as invalid, but is not proactively deleted.
func ValidateSession(ctx context.Context, db *sql.DB, token string) (userID int64, ok bool) {
	session, err := storage.GetSessionByTokenHash(db, hashSessionToken(token))
	if err != nil {
		return 0, false
	}

	if time.Now().Unix() > session.ExpiresAt {
		return 0, false
	}

	return session.UserID, true
}

// generateSessionToken returns a fresh cryptographically random session
// token, URL-safe base64 encoded.
func generateSessionToken() (string, error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashSessionToken returns the hex-encoded SHA-256 hash of a raw session
// token, as stored in storage.sessions.token_hash.
func hashSessionToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// baseURLIsSecure reports whether baseURL uses the https scheme.
func baseURLIsSecure(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return u.Scheme == "https"
}
