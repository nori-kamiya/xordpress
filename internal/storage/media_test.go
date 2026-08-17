package storage_test

import (
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestInsertAndGetMediaRoundTrip(t *testing.T) {
	db := newTestDB(t)

	id, err := storage.InsertMedia(db, "abc.jpg", "photo.jpg", "image/jpeg", 1024, 100, 200, 1)
	if err != nil {
		t.Fatalf("InsertMedia: %v", err)
	}
	if id == 0 {
		t.Fatalf("expected InsertMedia to return a nonzero id")
	}

	m, err := storage.GetMediaByID(db, id)
	if err != nil {
		t.Fatalf("GetMediaByID: %v", err)
	}
	if m.StoredName != "abc.jpg" || m.OriginalName != "photo.jpg" || m.MimeType != "image/jpeg" {
		t.Fatalf("GetMediaByID returned unexpected media: %+v", m)
	}
	if m.SizeBytes != 1024 || !m.Width.Valid || m.Width.Int64 != 100 || !m.Height.Valid || m.Height.Int64 != 200 {
		t.Fatalf("GetMediaByID returned unexpected media: %+v", m)
	}
	if m.UploaderID != 1 || m.CreatedAt == 0 {
		t.Fatalf("GetMediaByID returned unexpected media: %+v", m)
	}
}

func TestGetMediaByIDNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := storage.GetMediaByID(db, 999)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListMedia(t *testing.T) {
	db := newTestDB(t)

	// InsertMedia stamps created_at with time.Now(), so all three rows
	// would land in the same wall-clock second; seed created_at directly
	// to pin the DESC ordering deterministically (same escape hatch
	// newTestDB uses to seed users).
	for i, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		if _, err := db.Exec(
			`INSERT INTO media (stored_name, original_name, mime_type, size_bytes, width, height, uploader_id, created_at)
			 VALUES (?, ?, 'image/jpeg', 10, 1, 1, 1, ?)`, name, name, 1000+i,
		); err != nil {
			t.Fatalf("seed media(%s): %v", name, err)
		}
	}

	items, total, err := storage.ListMedia(db, 2, 0)
	if err != nil {
		t.Fatalf("ListMedia: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items with limit=2, got %d", len(items))
	}
	if items[0].StoredName != "c.jpg" || items[1].StoredName != "b.jpg" {
		t.Fatalf("expected newest-first order [c.jpg, b.jpg], got [%s, %s]", items[0].StoredName, items[1].StoredName)
	}

	offset, total, err := storage.ListMedia(db, 1, 1)
	if err != nil {
		t.Fatalf("ListMedia: %v", err)
	}
	if total != 3 || len(offset) != 1 || offset[0].StoredName != "b.jpg" {
		t.Fatalf("expected offset=1 to return second-newest [b.jpg], got total=%d items=%+v", total, offset)
	}

	all, total, err := storage.ListMedia(db, 0, 0)
	if err != nil {
		t.Fatalf("ListMedia: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("expected all 3 items with limit=0, got total=%d len=%d", total, len(all))
	}
}

func TestDeleteMedia(t *testing.T) {
	db := newTestDB(t)

	id, err := storage.InsertMedia(db, "abc.jpg", "photo.jpg", "image/jpeg", 1024, 100, 200, 1)
	if err != nil {
		t.Fatalf("InsertMedia: %v", err)
	}

	if err := storage.DeleteMedia(db, id); err != nil {
		t.Fatalf("DeleteMedia: %v", err)
	}

	_, err = storage.GetMediaByID(db, id)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteMediaNotFound(t *testing.T) {
	db := newTestDB(t)

	err := storage.DeleteMedia(db, 999)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
