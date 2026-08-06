// Package content: site settings.
package content

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// SiteSettings is the domain representation of the site's configuration.
type SiteSettings struct {
	Name string
}

// GetSiteSettings returns the current site settings. Before initial setup
// has run, no site_name row exists yet; this is not an error condition, so
// GetSiteSettings returns a zero-value SiteSettings in that case.
func GetSiteSettings(db *sql.DB) (SiteSettings, error) {
	name, err := storage.GetSetting(db, "site_name")
	if errors.Is(err, storage.ErrNotFound) {
		return SiteSettings{}, nil
	}
	if err != nil {
		return SiteSettings{}, fmt.Errorf("content: get site settings: %w", err)
	}
	return SiteSettings{Name: name}, nil
}

// SetSiteSettings updates the site's name setting.
func SetSiteSettings(db *sql.DB, name string) error {
	if err := storage.SetSetting(db, "site_name", name); err != nil {
		return fmt.Errorf("content: set site settings: %w", err)
	}
	return nil
}
