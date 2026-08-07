package adminapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/auth"
	"github.com/nori-kamiya/xordpress/internal/auth/totp"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

// enrollConfirmedTOTP seeds a confirmed TOTP registration for userID
// directly via storage (T13's enrollment API doesn't exist yet), returning
// the secret so tests can generate valid codes with totp.GenerateCode.
func enrollConfirmedTOTP(t *testing.T, db *sql.DB, userID int64) string {
	t.Helper()

	secret, err := totp.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if err := storage.UpsertUserTOTPSecret(db, userID, secret); err != nil {
		t.Fatalf("UpsertUserTOTPSecret: %v", err)
	}
	if err := storage.ConfirmUserTOTP(db, userID, 0); err != nil {
		t.Fatalf("ConfirmUserTOTP: %v", err)
	}
	return secret
}

func doVerifyMFARequest(t *testing.T, handler http.Handler, mfaToken, code string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{"mfa_token": mfaToken, "code": code})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/session/mfa", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestLogin_ConfirmedTOTP_ReturnsMFARequiredInsteadOfSession(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	userID, ok := currentUserIDByUsername(t, db, "admin")
	if !ok {
		t.Fatal("could not resolve seeded admin's id")
	}
	enrollConfirmedTOTP(t, db, userID)

	rec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("no session cookie should be set before 2FA completes")
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["mfa_required"] != true {
		t.Fatalf("resp = %v, want mfa_required=true", resp)
	}
	if _, ok := resp["mfa_token"].(string); !ok {
		t.Fatalf("resp = %v, want a string mfa_token", resp)
	}
}

func TestLogin_UnconfirmedTOTP_IssuesSessionDirectly(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	userID, _ := currentUserIDByUsername(t, db, "admin")

	secret, err := totp.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if err := storage.UpsertUserTOTPSecret(db, userID, secret); err != nil {
		t.Fatalf("UpsertUserTOTPSecret: %v", err)
	}

	rec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 1 {
		t.Fatal("an unconfirmed TOTP registration must not require 2FA at login")
	}
}

func TestVerifyMFA_CorrectCode_IssuesSessionAndConsumesChallenge(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	userID, _ := currentUserIDByUsername(t, db, "admin")
	secret := enrollConfirmedTOTP(t, db, userID)

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	var loginResp map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mfaToken := loginResp["mfa_token"].(string)

	code, err := totp.GenerateCode(secret, totp.CurrentStep(time.Now()))
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}

	rec := doVerifyMFARequest(t, adminapi.VerifyMFA(db, db, testBaseURL), mfaToken, code)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 1 {
		t.Fatal("a real session cookie should be issued after successful 2FA")
	}

	// The challenge is single-use.
	replayRec := doVerifyMFARequest(t, adminapi.VerifyMFA(db, db, testBaseURL), mfaToken, code)
	if replayRec.Code != http.StatusNotFound {
		t.Fatalf("replay status = %d, want %d (challenge already consumed)", replayRec.Code, http.StatusNotFound)
	}
}

func TestVerifyMFA_ReplayedCode_Returns401(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	userID, _ := currentUserIDByUsername(t, db, "admin")
	secret := enrollConfirmedTOTP(t, db, userID)

	step := totp.CurrentStep(time.Now())
	code, err := totp.GenerateCode(secret, step)
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if err := storage.UpdateUserTOTPLastUsedStep(db, userID, step); err != nil {
		t.Fatalf("UpdateUserTOTPLastUsedStep: %v", err)
	}

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	var loginResp map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mfaToken := loginResp["mfa_token"].(string)

	rec := doVerifyMFARequest(t, adminapi.VerifyMFA(db, db, testBaseURL), mfaToken, code)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestVerifyMFA_TenFailures_LocksOutFor15Minutes(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	userID, _ := currentUserIDByUsername(t, db, "admin")
	enrollConfirmedTOTP(t, db, userID)

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	var loginResp map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mfaToken := loginResp["mfa_token"].(string)

	var last *httptest.ResponseRecorder
	for i := 0; i < storage.MaxMFAFailures; i++ {
		last = doVerifyMFARequest(t, adminapi.VerifyMFA(db, db, testBaseURL), mfaToken, "000000")
	}
	if last.Code != http.StatusUnauthorized {
		t.Fatalf("last pre-lockout attempt status = %d, want %d, body=%s", last.Code, http.StatusUnauthorized, last.Body.String())
	}

	lockedRec := doVerifyMFARequest(t, adminapi.VerifyMFA(db, db, testBaseURL), mfaToken, "000000")
	if lockedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d, body=%s", lockedRec.Code, http.StatusTooManyRequests, lockedRec.Body.String())
	}
}

func TestVerifyMFA_UnknownToken_Returns404(t *testing.T) {
	db := newTestDB(t)

	rec := doVerifyMFARequest(t, adminapi.VerifyMFA(db, db, testBaseURL), "bogus-token", "123456")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestVerifyMFA_RecoveryCode_IssuesSessionAndConsumesCode(t *testing.T) {
	db := newTestDB(t)
	seedAdmin(t, db, "admin", "password123")
	userID, _ := currentUserIDByUsername(t, db, "admin")
	enrollConfirmedTOTP(t, db, userID)

	recoveryCode := "abcd-efgh-ijkl"
	hash, err := auth.HashPassword(recoveryCode)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := storage.InsertRecoveryCodes(db, userID, []string{hash}); err != nil {
		t.Fatalf("InsertRecoveryCodes: %v", err)
	}

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	var loginResp map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mfaToken := loginResp["mfa_token"].(string)

	rec := doVerifyMFARequest(t, adminapi.VerifyMFA(db, db, testBaseURL), mfaToken, recoveryCode)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	hashes, err := storage.UnusedRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("UnusedRecoveryCodeHashes: %v", err)
	}
	if len(hashes) != 0 {
		t.Fatalf("unused recovery codes = %v, want none left after use", hashes)
	}
}

// currentUserIDByUsername looks up a seeded user's id directly, since
// seedAdmin/CreateFirstAdmin don't return it.
func currentUserIDByUsername(t *testing.T, db *sql.DB, username string) (int64, bool) {
	t.Helper()
	u, err := storage.GetUserByUsername(db, username)
	if err != nil {
		return 0, false
	}
	return u.ID, true
}
