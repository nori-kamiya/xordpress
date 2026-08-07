// Package adminapi: TOTP self-enrollment/disable (design.md §5, §6.1).
// CapSelfManage throughout: a user manages their own 2FA, never another
// user's (that lockout-recovery path is DeleteUserMFA, CapUsersManage).
package adminapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/auth/totp"
	"github.com/nori-kamiya/xordpress/internal/content"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// recoveryCodeCount is how many recovery codes are generated on
// confirmation (design.md §5).
const recoveryCodeCount = 10

// totpIssuer falls back to when no site name has been configured yet
// (GetSiteSettings' zero value), so the otpauth:// URI's label is never
// blank.
const totpIssuerFallback = "XordPress"

// verifyAccountPassword resolves the requesting Principal and checks
// password against their stored hash, the shared precondition for all
// three /api/account/mfa* endpoints (design.md §5: each requires the
// current password so a hijacked, still-logged-in session cannot silently
// take over 2FA). Returns ok=false after already writing the appropriate
// error response.
func verifyAccountPassword(w http.ResponseWriter, r *http.Request, contentDB, authDB *sql.DB, password string) (userID int64, username string, ok bool) {
	p, found := currentPrincipal(r, authDB)
	if !found {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return 0, "", false
	}

	user, err := storage.GetUserByID(contentDB, p.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return 0, "", false
	}
	if !auth.VerifyPassword(user.PasswordHash, password) {
		writeError(w, http.StatusUnauthorized, "incorrect password")
		return 0, "", false
	}
	return user.ID, user.Username, true
}

// startTOTPEnrollmentRequest is the JSON body of POST /api/account/mfa.
type startTOTPEnrollmentRequest struct {
	Password string `json:"password"`
}

// StartTOTPEnrollment returns the handler for POST /api/account/mfa
// (design.md §5): begins TOTP enrollment, returning a fresh secret and its
// otpauth:// URI. The registration is unconfirmed until ConfirmTOTPEnrollment
// succeeds, so it does not yet require 2FA at login (design.md §6.1).
//
// If the caller already has a confirmed registration, this responds 409
// instead of replacing it: user_totp.user_id is a single-row-per-user
// primary key, so a second enrollment can only overwrite the first, which
// would (a) leave the previous enrollment's recovery codes valid against a
// secret the user no longer holds, and (b) if abandoned before confirming,
// silently drop the account back to no-2FA (confirmed_at reset to NULL) with
// no visible signal. DisableTOTP already tears down user_totp, recovery
// codes, failures, and challenges together, so re-enrollment goes through
// disable-then-start rather than an implicit overwrite.
func StartTOTPEnrollment(contentDB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req startTOTPEnrollmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		userID, username, ok := verifyAccountPassword(w, r, contentDB, authDB, req.Password)
		if !ok {
			return
		}

		existing, err := storage.GetUserTOTP(contentDB, userID)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err == nil && existing.ConfirmedAt != nil {
			writeError(w, http.StatusConflict, "2FA is already enabled; disable it before re-enrolling")
			return
		}

		secret, err := totp.GenerateSecret()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := storage.UpsertUserTOTPSecret(contentDB, userID, secret); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		settings, err := content.GetSiteSettings(contentDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		issuer := settings.Name
		if issuer == "" {
			issuer = totpIssuerFallback
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"secret":      secret,
			"otpauth_uri": totp.OTPAuthURI(secret, username, issuer),
		})
	})
}

// confirmTOTPEnrollmentRequest is the JSON body of
// POST /api/account/mfa/confirm.
type confirmTOTPEnrollmentRequest struct {
	Code string `json:"code"`
}

// ConfirmTOTPEnrollment returns the handler for
// POST /api/account/mfa/confirm (design.md §5): verifies code against the
// pending secret from StartTOTPEnrollment, activates it, and generates
// recovery codes — returned in plaintext exactly once, since only their
// bcrypt hashes are persisted.
func ConfirmTOTPEnrollment(contentDB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := currentPrincipal(r, authDB)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req confirmTOTPEnrollmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		reg, err := storage.GetUserTOTP(contentDB, p.UserID)
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no pending enrollment")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		step, valid := totp.Validate(reg.Secret, req.Code, totp.CurrentStep(time.Now()))
		if !valid {
			writeError(w, http.StatusUnauthorized, "invalid code")
			return
		}

		if err := storage.ConfirmUserTOTP(contentDB, p.UserID, step); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		codes, hashes, err := generateRecoveryCodes()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := storage.InsertRecoveryCodes(contentDB, p.UserID, hashes); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
	})
}

// disableTOTPRequest is the JSON body of DELETE /api/account/mfa.
type disableTOTPRequest struct {
	Password string `json:"password"`
}

// DisableTOTP returns the handler for DELETE /api/account/mfa (design.md
// §5): clears the caller's own TOTP registration, recovery codes, and any
// in-progress lockout/challenge state, requiring the current password like
// the other two endpoints.
func DisableTOTP(contentDB, authDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req disableTOTPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		userID, _, ok := verifyAccountPassword(w, r, contentDB, authDB, req.Password)
		if !ok {
			return
		}

		tx, err := contentDB.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		defer tx.Rollback()

		if err := storage.ResetUserMFATx(tx, userID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// generateRecoveryCodes returns recoveryCodeCount fresh recovery codes in
// both plaintext (to return to the caller once) and bcrypt-hashed form (to
// persist), in matching order.
func generateRecoveryCodes() (codes []string, hashes []string, err error) {
	for i := 0; i < recoveryCodeCount; i++ {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		hash, err := auth.HashPassword(code)
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, hash)
	}
	return codes, hashes, nil
}

// generateRecoveryCode returns a single random recovery code, formatted as
// two 5-character hex groups (e.g. "3f9a1-2b7c4") for readability when
// hand-typed.
func generateRecoveryCode() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	full := hex.EncodeToString(buf)
	return fmt.Sprintf("%s-%s", full[:5], full[5:]), nil
}
