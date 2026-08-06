// Package content: first-admin setup.
package content

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// CreateFirstAdmin creates the first admin user and records siteName as the
// site's name, atomically. It returns created=true if the admin user was
// created, or created=false without error if a user already existed.
func CreateFirstAdmin(ctx context.Context, db *sql.DB, username, passwordHash, siteName string) (bool, error) {
	created, err := storage.CreateFirstAdmin(ctx, db, username, passwordHash, siteName)
	if err != nil {
		return false, fmt.Errorf("content: create first admin: %w", err)
	}
	return created, nil
}
