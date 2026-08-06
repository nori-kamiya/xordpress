package content_test

import (
	"testing"

	"github.com/nori-kamiya/xordpress/internal/content"
)

func TestGetSiteSettingsNotFound(t *testing.T) {
	db := newTestDB(t)

	settings, err := content.GetSiteSettings(db)
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if settings.Name != "" {
		t.Fatalf("expected zero-value Name before setup, got %q", settings.Name)
	}
}

func TestSetAndGetSiteSettingsRoundTrip(t *testing.T) {
	db := newTestDB(t)

	if err := content.SetSiteSettings(db, "xordpress"); err != nil {
		t.Fatalf("SetSiteSettings: %v", err)
	}

	got, err := content.GetSiteSettings(db)
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if got.Name != "xordpress" {
		t.Fatalf("expected %q, got %q", "xordpress", got.Name)
	}
}

func TestSetSiteSettingsUpserts(t *testing.T) {
	db := newTestDB(t)

	if err := content.SetSiteSettings(db, "first"); err != nil {
		t.Fatalf("SetSiteSettings (first): %v", err)
	}
	if err := content.SetSiteSettings(db, "second"); err != nil {
		t.Fatalf("SetSiteSettings (second): %v", err)
	}

	got, err := content.GetSiteSettings(db)
	if err != nil {
		t.Fatalf("GetSiteSettings: %v", err)
	}
	if got.Name != "second" {
		t.Fatalf("expected upsert to overwrite value, got %q", got.Name)
	}
}
