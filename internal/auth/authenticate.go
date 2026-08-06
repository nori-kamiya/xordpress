// Package auth: user authentication.
package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// ErrAccountLocked is returned by Authenticate when username has exceeded
// storage.MaxLoginFailures consecutive failed attempts and is still within
// storage.LoginLockoutDuration of the most recent one (roadmap-to-wp-parity.md
// P0-3: brute-force lockout). Callers should surface this distinctly from a
// plain wrong-password result (ok=false, err=nil) so the client can show
// "try again later" rather than "wrong password".
var ErrAccountLocked = errors.New("auth: account temporarily locked due to too many failed login attempts")

// Authenticate verifies the provided username and password against the stored
// user credentials. Returns the user ID if authentication succeeds, ok=true if
// credentials are valid, ok=false if user not found or password is incorrect.
// err is returned for database errors, and as ErrAccountLocked (with ok=false)
// if the account is currently locked out from too many recent failures.
//
// Lockout is keyed by username and only tracked for usernames that actually
// exist, so probing nonexistent usernames cannot grow the login_failures
// table and cannot lock out a real account.
func Authenticate(ctx context.Context, db *sql.DB, username, password string) (userID int64, ok bool, err error) {
	user, err := storage.GetUserByUsername(db, username)
	if err == storage.ErrNotFound {
		// User does not exist - return ok=false without error
		return 0, false, nil
	}
	if err != nil {
		// Database error
		return 0, false, err
	}

	locked, err := storage.IsLoginLocked(db, username)
	if err != nil {
		return 0, false, err
	}
	if locked {
		return 0, false, ErrAccountLocked
	}

	// User found, verify password
	if !VerifyPassword(user.PasswordHash, password) {
		if err := storage.RecordLoginFailure(db, username); err != nil {
			return 0, false, err
		}
		return 0, false, nil
	}

	if err := storage.ResetLoginFailures(db, username); err != nil {
		return 0, false, err
	}

	// Credentials are valid
	return user.ID, true, nil
}
