package adminapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// newRoutesTestDB returns a migrated in-memory SQLite DB. Kept local to this
// file (rather than reusing adminapi_test's newTestDB) because route/
// requireCap are unexported and this file must live in package adminapi to
// reach them.
func newRoutesTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRoute_PanicsWithoutCapability(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("route() did not panic for cap == CapNone")
		}
	}()
	route("GET /api/whatever", auth.CapNone, okHandler())
}

func TestRoutes_EveryRouteHasACapability(t *testing.T) {
	db := newRoutesTestDB(t)
	for _, rt := range routes(db, db) {
		if rt.cap == auth.CapNone {
			t.Errorf("route %q has CapNone", rt.pattern)
		}
	}
}

func TestRequireCap_NoPrincipalUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	rec := httptest.NewRecorder()

	requireCap(auth.CapPostsRead, okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireCap_PrincipalLacksCapabilityForbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/posts/1", nil)
	ctx := auth.WithPrincipal(req.Context(), auth.Principal{UserID: 1, Role: auth.RoleContributor})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	requireCap(auth.CapUsersManage, okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireCap_PrincipalHasCapabilityPasses(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/posts", nil)
	ctx := auth.WithPrincipal(req.Context(), auth.Principal{UserID: 1, Role: auth.RoleContributor})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	requireCap(auth.CapPostsRead, okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
