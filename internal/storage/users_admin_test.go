package storage_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage"
)

func TestCreateUserTx_DuplicateUsername_ReturnsErrUsernameTaken(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if _, err := storage.CreateUserTx(tx, "admin", "hash", "editor"); !errors.Is(err, storage.ErrUsernameTaken) {
		t.Fatalf("CreateUserTx() error = %v, want ErrUsernameTaken", err)
	}
}

func TestCreateUserTx_ThenListUsers_IncludesNewUser(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	id, err := storage.CreateUserTx(tx, "editor-1", "hash", "editor")
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	users, err := storage.ListUsers(db)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("ListUsers() len = %d, want 2", len(users))
	}

	var found *storage.AdminUser
	for _, u := range users {
		if u.ID == id {
			found = u
		}
	}
	if found == nil {
		t.Fatalf("ListUsers() did not include newly created user id=%d", id)
	}
	if found.Username != "editor-1" || found.Role != "editor" {
		t.Errorf("found = %+v, want Username=editor-1 Role=editor", found)
	}
}

func TestGetUserByIDTx_NotFound(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if _, err := storage.GetUserByIDTx(tx, 999); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetUserByIDTx() error = %v, want ErrNotFound", err)
	}
}

// TestUpdateUserRoleTx_UpdatesRole demotes user 1, acting as a second
// administrator (user 2) so the demotion does not trip the last-admin
// guard.
func TestUpdateUserRoleTx_UpdatesRole(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	actorID, err := storage.CreateUserTx(tx, "admin-2", "hash", "administrator")
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}
	if err := storage.UpdateUserRoleTx(tx, 1, "editor", actorID); err != nil {
		t.Fatalf("UpdateUserRoleTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	users, err := storage.ListUsers(db)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if users[0].Role != "editor" {
		t.Errorf("role = %q, want %q", users[0].Role, "editor")
	}
}

func TestUpdateUserRoleTx_NotFound(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := storage.UpdateUserRoleTx(tx, 999, "editor", 1); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("UpdateUserRoleTx() error = %v, want ErrNotFound", err)
	}
}

func TestLastAdminProtection_SelfDemote_ReturnsErrSelfTarget(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := storage.UpdateUserRoleTx(tx, 1, "editor", 1); !errors.Is(err, storage.ErrSelfTarget) {
		t.Fatalf("UpdateUserRoleTx() error = %v, want ErrSelfTarget", err)
	}
}

// TestLastAdminProtection_DemoteSoleAdmin_ReturnsErrLastAdmin demotes the sole
// administrator (user 1), acting as a non-administrator second user, and
// expects ErrLastAdmin.
func TestLastAdminProtection_DemoteSoleAdmin_ReturnsErrLastAdmin(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	actorID, err := storage.CreateUserTx(tx, "editor-1", "hash", "editor")
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}

	if err := storage.UpdateUserRoleTx(tx, 1, "editor", actorID); !errors.Is(err, storage.ErrLastAdmin) {
		t.Fatalf("UpdateUserRoleTx() error = %v, want ErrLastAdmin", err)
	}
}

func TestCountAdministratorsExceptTx(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	// newTestDB seeds one administrator (id=1, DEFAULT role).
	count, err := storage.CountAdministratorsExceptTx(tx, 1)
	if err != nil {
		t.Fatalf("CountAdministratorsExceptTx: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (excludeID is the only administrator)", count)
	}

	if _, err := storage.CreateUserTx(tx, "admin-2", "hash", "administrator"); err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}

	count, err = storage.CountAdministratorsExceptTx(tx, 1)
	if err != nil {
		t.Fatalf("CountAdministratorsExceptTx: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (a second administrator now exists)", count)
	}
}

func TestDeleteUserCascadeTx_SelfReassign_ReturnsError(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := storage.DeleteUserCascadeTx(tx, 1, 1, 2); !errors.Is(err, storage.ErrSelfReassign) {
		t.Fatalf("DeleteUserCascadeTx() error = %v, want ErrSelfReassign", err)
	}
}

func TestDeleteUserCascadeTx_ReassignTargetNotFound_ReturnsError(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := storage.DeleteUserCascadeTx(tx, 1, 999, 2); !errors.Is(err, storage.ErrReassignTargetNotFound) {
		t.Fatalf("DeleteUserCascadeTx() error = %v, want ErrReassignTargetNotFound", err)
	}
}

// TestDeleteUserCascadeTx_ReassignTargetNotFoundTakesPrecedenceOverLastAdmin
// pins the documented check order (godoc on DeleteUserCascadeTx): id=1 is
// the sole administrator (would trigger ErrLastAdmin) AND reassignTo=999
// does not exist (would trigger ErrReassignTargetNotFound). The structural
// validation error must win so a caller always learns about a bad
// reassign_to before a business-state conflict.
func TestDeleteUserCascadeTx_ReassignTargetNotFoundTakesPrecedenceOverLastAdmin(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := storage.DeleteUserCascadeTx(tx, 1, 999, 2); !errors.Is(err, storage.ErrReassignTargetNotFound) {
		t.Fatalf("DeleteUserCascadeTx() error = %v, want ErrReassignTargetNotFound (not ErrLastAdmin, even though id=1 is the sole administrator)", err)
	}
}

func TestDeleteUserCascadeTx_UserNotFound_ReturnsError(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := storage.DeleteUserCascadeTx(tx, 999, 1, 2); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("DeleteUserCascadeTx() error = %v, want ErrNotFound", err)
	}
}

func TestLastAdminProtection_SelfDelete_ReturnsErrSelfTarget(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	if err := storage.DeleteUserCascadeTx(tx, 1, 2, 1); !errors.Is(err, storage.ErrSelfTarget) {
		t.Fatalf("DeleteUserCascadeTx() error = %v, want ErrSelfTarget", err)
	}
}

// TestLastAdminProtection_DeleteSoleAdmin_ReturnsErrLastAdmin deletes the sole
// administrator (user 1), acting as a non-administrator second user, and
// expects ErrLastAdmin.
func TestLastAdminProtection_DeleteSoleAdmin_ReturnsErrLastAdmin(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	actorID, err := storage.CreateUserTx(tx, "editor-1", "hash", "editor")
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}

	if err := storage.DeleteUserCascadeTx(tx, 1, actorID, actorID); !errors.Is(err, storage.ErrLastAdmin) {
		t.Fatalf("DeleteUserCascadeTx() error = %v, want ErrLastAdmin", err)
	}
}

// TestLastAdminProtection_SecondAdminExists_DeleteSucceeds deletes an
// administrator when a second administrator exists, and expects success.
func TestLastAdminProtection_SecondAdminExists_DeleteSucceeds(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	actorID, err := storage.CreateUserTx(tx, "admin-2", "hash", "administrator")
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}

	if err := storage.DeleteUserCascadeTx(tx, 1, actorID, actorID); err != nil {
		t.Fatalf("DeleteUserCascadeTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM users WHERE id = 1`).Scan(&count); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if count != 0 {
		t.Error("user 1 was not deleted")
	}
}

// TestDeleteUserCascadeTx_RemovesAllRelatedRows seeds a second user with a
// row in every table design.md §6.2 lists, deletes them, and asserts no
// trace remains in any of those tables (and that their post was reassigned
// rather than orphaned/deleted).
func TestDeleteUserCascadeTx_RemovesAllRelatedRows(t *testing.T) {
	db := newTestDB(t)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	targetID, err := storage.CreateUserTx(tx, "target", "hash", "contributor")
	if err != nil {
		t.Fatalf("CreateUserTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	seedRelatedRows(t, db, targetID)

	var postID int64
	if err := db.QueryRow(
		`INSERT INTO posts (slug, title, body, status, author_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 0, 0) RETURNING id`,
		"target-post", "Target's post", "<p>x</p>", "draft", targetID,
	).Scan(&postID); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := storage.DeleteUserCascadeTx(tx, targetID, 1, 1); err != nil {
		t.Fatalf("DeleteUserCascadeTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM users WHERE id = ?`, targetID).Scan(&count); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if count != 0 {
		t.Error("users: target row was not deleted")
	}

	for _, tbl := range []string{"sessions", "login_failures", "user_totp", "mfa_recovery_codes", "mfa_failures", "mfa_challenges"} {
		var n int
		var q string
		if tbl == "login_failures" {
			q = `SELECT count(*) FROM login_failures WHERE username = 'target'`
		} else {
			q = "SELECT count(*) FROM " + tbl + " WHERE user_id = ?"
		}
		var qerr error
		if tbl == "login_failures" {
			qerr = db.QueryRow(q).Scan(&n)
		} else {
			qerr = db.QueryRow(q, targetID).Scan(&n)
		}
		if qerr != nil {
			t.Fatalf("query %s: %v", tbl, qerr)
		}
		if n != 0 {
			t.Errorf("%s: %d rows remain for deleted user, want 0", tbl, n)
		}
	}

	var authorID int64
	if err := db.QueryRow(`SELECT author_id FROM posts WHERE id = ?`, postID).Scan(&authorID); err != nil {
		t.Fatalf("query post author: %v", err)
	}
	if authorID != 1 {
		t.Errorf("post author_id = %d, want 1 (reassigned)", authorID)
	}
}

// seedRelatedRows inserts one row into every table design.md §6.2's
// cascade must clear, keyed to userID (or its username for login_failures).
func seedRelatedRows(t *testing.T, db *sql.DB, userID int64) {
	t.Helper()

	if _, err := db.Exec(`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, 0, 0)`, "session-hash", userID); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO login_failures (username, fail_count, last_failed_at) VALUES ('target', 1, 0)`); err != nil {
		t.Fatalf("seed login_failures: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_totp (user_id, secret, confirmed_at) VALUES (?, 'secret', 0)`, userID); err != nil {
		t.Fatalf("seed user_totp: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO mfa_recovery_codes (user_id, code_hash) VALUES (?, 'code-hash')`, userID); err != nil {
		t.Fatalf("seed mfa_recovery_codes: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO mfa_failures (user_id, fail_count, last_failed_at) VALUES (?, 1, 0)`, userID); err != nil {
		t.Fatalf("seed mfa_failures: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO mfa_challenges (token_hash, user_id, expires_at) VALUES ('challenge-hash', ?, 0)`, userID); err != nil {
		t.Fatalf("seed mfa_challenges: %v", err)
	}
}
