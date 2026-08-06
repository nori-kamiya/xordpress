// Package auth: user authentication.
package auth

import (
	"context"
	"database/sql"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// Authenticate verifies the provided username and password against the stored
// user credentials. Returns the user ID if authentication succeeds, ok=true if
// credentials are valid, ok=false if user not found or password is incorrect.
// err is returned only for database errors.
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

	// User found, verify password
	if !VerifyPassword(user.PasswordHash, password) {
		// Password does not match
		return 0, false, nil
	}

	// Credentials are valid
	return user.ID, true, nil
}
