// Package adminapi: second factor of the login flow (design.md §6.1).
package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/auth/totp"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// verifyMFARequest is the JSON body of POST /api/session/mfa.
type verifyMFARequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

// VerifyMFA returns the handler for POST /api/session/mfa (design.md §6.1
// steps 3-5), the second half of a 2FA login. mfa_token must be the raw
// value POST /api/session returned; code is either a live TOTP code or an
// unused recovery code.
func VerifyMFA(contentDB, authDB *sql.DB, baseURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req verifyMFARequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		challenge, err := storage.GetMFAChallenge(authDB, hashSessionToken(req.MFAToken))
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		userID := challenge.UserID

		// Re-checked here (not just at challenge issuance): time may have
		// passed since POST /api/session, and skipping this check would let
		// the last attempt of a lockout period slip through (design.md
		// §6.1 step 3).
		locked, err := storage.IsMFALocked(authDB, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if locked {
			writeError(w, http.StatusTooManyRequests, "too many failed 2FA attempts, try again later")
			return
		}

		reg, err := storage.GetUserTOTP(contentDB, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if reg.ConfirmedAt == nil {
			// The registration backing this challenge was replaced by a new,
			// not-yet-confirmed enrollment (StartTOTPEnrollment) after the
			// challenge was issued. Validating against the new secret here
			// would accept a code the user's authenticator app was never
			// shown for this login. Fail closed rather than silently
			// re-checking against unconfirmed state.
			writeError(w, http.StatusUnauthorized, "invalid code")
			return
		}

		verified, usedTOTPStep, recoveryHash, err := verifyMFACode(contentDB, reg, req.Code)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !verified {
			if err := storage.RecordMFAFailure(authDB, userID); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid code")
			return
		}

		if err := storage.DeleteMFAChallenge(authDB, hashSessionToken(req.MFAToken)); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if usedTOTPStep != nil {
			if err := storage.UpdateUserTOTPLastUsedStep(contentDB, userID, *usedTOTPStep); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
		if recoveryHash != "" {
			if err := storage.MarkRecoveryCodeUsed(contentDB, userID, recoveryHash); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
		if err := storage.ResetMFAFailures(authDB, userID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		cookie, err := auth.IssueSession(r.Context(), authDB, baseURL, userID, r.UserAgent(), clientIP(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
}

// verifyMFACode tries code first as a live TOTP code (rejecting a replay of
// reg.LastUsedStep or earlier) and, failing that, as an unused recovery
// code. Exactly one of usedTOTPStep/recoveryHash is set on success, telling
// the caller which side-effect to apply. err is a storage failure looking
// up recovery codes, distinct from "code did not verify" — the caller must
// not count it as a failed attempt toward the lockout.
func verifyMFACode(contentDB *sql.DB, reg *storage.UserTOTP, code string) (ok bool, usedTOTPStep *int64, recoveryHash string, err error) {
	if step, valid := totp.Validate(reg.Secret, code, totp.CurrentStep(time.Now())); valid && step > reg.LastUsedStep {
		return true, &step, "", nil
	}

	hashes, err := storage.UnusedRecoveryCodeHashes(contentDB, reg.UserID)
	if err != nil {
		return false, nil, "", err
	}
	for _, hash := range hashes {
		if auth.VerifyPassword(hash, code) {
			return true, nil, hash, nil
		}
	}
	return false, nil, "", nil
}
