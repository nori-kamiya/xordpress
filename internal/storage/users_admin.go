// Package storage: user CRUD for the admin API (roadmap-to-wp-parity.md
// P1-6, design.md §6.2). All mutations run under a caller-provided *sql.Tx
// — never mixed with *sql.DB calls in the same operation — so multi-step
// invariants (last-administrator protection in particular, added in T6)
// hold under SetMaxOpenConns(1)'s single-connection serialization.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
)

var (
	// ErrUsernameTaken is returned by CreateUserTx when username already
	// exists (users.username is UNIQUE).
	ErrUsernameTaken = errors.New("storage: username already taken")

	// ErrSelfReassign is returned by DeleteUserCascadeTx when reassignTo
	// equals the id being deleted.
	ErrSelfReassign = errors.New("storage: cannot reassign posts to the user being deleted")

	// ErrReassignTargetNotFound is returned by DeleteUserCascadeTx when
	// reassignTo does not name an existing user.
	ErrReassignTargetNotFound = errors.New("storage: reassign target user not found")

	// ErrLastAdmin is returned by DeleteUserCascadeTx and UpdateUserRoleTx
	// when the operation would leave the installation with zero
	// administrators.
	ErrLastAdmin = errors.New("storage: cannot remove the last administrator")

	// ErrSelfTarget is returned by DeleteUserCascadeTx and UpdateUserRoleTx
	// when id == actorID: self-deletion/self-role-change must go through
	// the self-service account endpoints (CapSelfManage), not the user
	// management endpoints (CapUsersManage).
	ErrSelfTarget = errors.New("storage: cannot target your own account")
)

// AdminUser is the persisted representation of a user row including role,
// as needed by the user management API (unlike User, which predates roles).
type AdminUser struct {
	ID        int64
	Username  string
	Role      string
	CreatedAt int64
}

// CreateUserTx inserts a new user row with the given role. Returns
// ErrUsernameTaken if username already exists.
func CreateUserTx(tx *sql.Tx, username, passwordHash, role string) (int64, error) {
	res, err := tx.Exec(
		`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, role, time.Now().Unix(),
	)
	if err != nil {
		var sqliteErr *sqlite.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqliteConstraintUnique {
			return 0, ErrUsernameTaken
		}
		return 0, fmt.Errorf("storage: create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("storage: create user: %w", err)
	}
	return id, nil
}

// scanAdminUsers reads every row of rows into AdminUsers, closing rows
// before returning.
func scanAdminUsers(rows *sql.Rows) ([]*AdminUser, error) {
	defer rows.Close()

	var users []*AdminUser
	for rows.Next() {
		u := &AdminUser{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("storage: scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list users: %w", err)
	}
	return users, nil
}

// ListUsersTx returns every user, ordered by id.
func ListUsersTx(tx *sql.Tx) ([]*AdminUser, error) {
	rows, err := tx.Query(`SELECT id, username, role, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage: list users: %w", err)
	}
	return scanAdminUsers(rows)
}

// ListUsers returns every user, ordered by id.
func ListUsers(db *sql.DB) ([]*AdminUser, error) {
	rows, err := db.Query(`SELECT id, username, role, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage: list users: %w", err)
	}
	return scanAdminUsers(rows)
}

// GetUserByIDTx returns the user with the given id. Returns ErrNotFound if
// no such user exists.
func GetUserByIDTx(tx *sql.Tx, id int64) (*AdminUser, error) {
	row := tx.QueryRow(`SELECT id, username, role, created_at FROM users WHERE id = ?`, id)

	u := &AdminUser{}
	err := row.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get user by id: %w", err)
	}
	return u, nil
}

// UpdateUserRoleTx sets the role for the user with the given id, acting on
// behalf of actorID. Returns ErrSelfTarget if id == actorID, ErrNotFound if
// no such user exists, and ErrLastAdmin if id is currently the sole
// administrator and role is not "administrator" (design.md §6.2: this
// check and the UPDATE run in the same transaction so two concurrent
// demotions of two different administrators cannot both observe "another
// administrator exists" and leave zero).
func UpdateUserRoleTx(tx *sql.Tx, id int64, role string, actorID int64) error {
	if id == actorID {
		return ErrSelfTarget
	}

	var currentRole string
	err := tx.QueryRow(`SELECT role FROM users WHERE id = ?`, id).Scan(&currentRole)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("storage: update user role: lookup current role: %w", err)
	}

	if currentRole == "administrator" && role != "administrator" {
		count, err := CountAdministratorsExceptTx(tx, id)
		if err != nil {
			return fmt.Errorf("storage: update user role: count administrators: %w", err)
		}
		if count == 0 {
			return ErrLastAdmin
		}
	}

	if _, err := tx.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id); err != nil {
		return fmt.Errorf("storage: update user role: %w", err)
	}
	return nil
}

// CountAdministratorsExceptTx returns the number of users with role
// 'administrator' other than excludeID. Used to guard against removing the
// last administrator (T6).
func CountAdministratorsExceptTx(tx *sql.Tx, excludeID int64) (int, error) {
	var count int
	if err := tx.QueryRow(
		`SELECT count(*) FROM users WHERE role = 'administrator' AND id != ?`, excludeID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("storage: count administrators except: %w", err)
	}
	return count, nil
}

// DeleteUserCascadeTx deletes the user with the given id, acting on behalf
// of actorID, reassigning their posts to reassignTo first (design.md §6.2:
// posts.author_id and sessions.user_id are REFERENCES users(id) with the
// default RESTRICT action, so deleting a user without reassigning their
// posts fails with a foreign key violation).
//
// Checks run in this order, and the first failing one wins: ErrSelfTarget
// (id == actorID), ErrSelfReassign (reassignTo == id), ErrNotFound (id does
// not name an existing user), ErrReassignTargetNotFound (reassignTo does
// not name an existing user), ErrLastAdmin (id is the sole administrator).
func DeleteUserCascadeTx(tx *sql.Tx, id int64, reassignTo int64, actorID int64) error {
	if id == actorID {
		return ErrSelfTarget
	}
	if reassignTo == id {
		return ErrSelfReassign
	}

	var username, role string
	err := tx.QueryRow(`SELECT username, role FROM users WHERE id = ?`, id).Scan(&username, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("storage: delete user cascade: lookup user: %w", err)
	}

	var reassignExists int
	if err := tx.QueryRow(`SELECT count(*) FROM users WHERE id = ?`, reassignTo).Scan(&reassignExists); err != nil {
		return fmt.Errorf("storage: delete user cascade: check reassign target: %w", err)
	}
	if reassignExists == 0 {
		return ErrReassignTargetNotFound
	}

	if role == "administrator" {
		count, err := CountAdministratorsExceptTx(tx, id)
		if err != nil {
			return fmt.Errorf("storage: delete user cascade: count administrators: %w", err)
		}
		if count == 0 {
			return ErrLastAdmin
		}
	}

	if _, err := tx.Exec(`UPDATE posts SET author_id = ? WHERE author_id = ?`, reassignTo, id); err != nil {
		return fmt.Errorf("storage: delete user cascade: reassign posts: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("storage: delete user cascade: delete sessions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM login_failures WHERE username = ?`, username); err != nil {
		return fmt.Errorf("storage: delete user cascade: delete login failures: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM user_totp WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("storage: delete user cascade: delete user_totp: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM mfa_recovery_codes WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("storage: delete user cascade: delete mfa_recovery_codes: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM mfa_failures WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("storage: delete user cascade: delete mfa_failures: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM mfa_challenges WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("storage: delete user cascade: delete mfa_challenges: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		return fmt.Errorf("storage: delete user cascade: delete user: %w", err)
	}
	return nil
}
