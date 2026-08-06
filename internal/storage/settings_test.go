package storage_test

import (
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestGetSettingNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := storage.GetSetting(db, "site_name")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSetAndGetSettingRoundTrip(t *testing.T) {
	db := newTestDB(t)

	if err := storage.SetSetting(db, "site_name", "xordpress"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	v, err := storage.GetSetting(db, "site_name")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if v != "xordpress" {
		t.Fatalf("expected %q, got %q", "xordpress", v)
	}
}

func TestSetSettingUpserts(t *testing.T) {
	db := newTestDB(t)

	if err := storage.SetSetting(db, "site_name", "first"); err != nil {
		t.Fatalf("SetSetting (first): %v", err)
	}
	if err := storage.SetSetting(db, "site_name", "second"); err != nil {
		t.Fatalf("SetSetting (second): %v", err)
	}

	v, err := storage.GetSetting(db, "site_name")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if v != "second" {
		t.Fatalf("expected upsert to overwrite value, got %q", v)
	}
}
