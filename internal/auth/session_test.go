package auth

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

// newTestDB returns a migrated in-memory SQLite DB with one user (id=1).
func newTestDB(t *testing.T) *sql.DB {
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

	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, created_at) VALUES (1, 'admin', 'hash', 0)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	return db
}

func TestIssueSessionThenValidateSessionRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	cookie, err := IssueSession(ctx, db, "https://example.com", 1)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	if cookie.Name != sessionCookieName {
		t.Errorf("cookie name = %q, want %q", cookie.Name, sessionCookieName)
	}
	if cookie.Path != "/" {
		t.Errorf("cookie path = %q, want /", cookie.Path)
	}
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want SameSiteLaxMode", cookie.SameSite)
	}
	if !cookie.Secure {
		t.Error("cookie should be Secure for an https baseURL")
	}

	userID, ok := ValidateSession(ctx, db, cookie.Value)
	if !ok {
		t.Fatal("ValidateSession returned ok=false for a freshly issued session")
	}
	if userID != 1 {
		t.Errorf("userID = %d, want 1", userID)
	}
}

func TestValidateSessionWithRoleReturnsPrincipal(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	cookie, err := IssueSession(ctx, db, "https://example.com", 1)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	p, ok := ValidateSessionWithRole(ctx, db, cookie.Value)
	if !ok {
		t.Fatal("ValidateSessionWithRole returned ok=false for a freshly issued session")
	}
	if p.UserID != 1 {
		t.Errorf("UserID = %d, want 1", p.UserID)
	}
	if p.Role != RoleAdministrator {
		t.Errorf("Role = %q, want %q", p.Role, RoleAdministrator)
	}
}

func TestValidateSessionWithRoleWrongTokenFails(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if _, err := IssueSession(ctx, db, "http://localhost:8080", 1); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	_, ok := ValidateSessionWithRole(ctx, db, "not-a-real-token")
	if ok {
		t.Fatal("ValidateSessionWithRole returned ok=true for a token that was never issued")
	}
}

func TestIssueSessionSecureFlagFollowsBaseURLScheme(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	cookie, err := IssueSession(ctx, db, "http://localhost:8080", 1)
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	if cookie.Secure {
		t.Error("cookie should not be Secure for an http baseURL")
	}
}

func TestValidateSessionWrongTokenFails(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if _, err := IssueSession(ctx, db, "http://localhost:8080", 1); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	_, ok := ValidateSession(ctx, db, "not-a-real-token")
	if ok {
		t.Fatal("ValidateSession returned ok=true for a token that was never issued")
	}
}

func TestValidateSessionExpiredFails(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Manually insert an already-expired session, bypassing IssueSession, so
	// we can assert on lazy expiry without waiting 7 days.
	const rawToken = "expired-token"
	pastExpiry := time.Now().Add(-1 * time.Hour).Unix()

	if err := storage.InsertSession(db, hashSessionToken(rawToken), 1, pastExpiry); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	_, ok := ValidateSession(ctx, db, rawToken)
	if ok {
		t.Fatal("ValidateSession returned ok=true for an expired session")
	}
}
