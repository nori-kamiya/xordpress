package adminapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nori-kamiya/xordpress/internal/adminapi"
	"github.com/nori-kamiya/xordpress/internal/auth/totp"
	"github.com/nori-kamiya/xordpress/internal/storage"
)

func doStartTOTPEnrollmentRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/account/mfa", bytes.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func doConfirmTOTPEnrollmentRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, code string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/account/mfa/confirm", bytes.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func doDisableTOTPRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/account/mfa", bytes.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestTOTPEnroll_StartThenConfirm_ActivatesAndReturnsRecoveryCodes(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)

	startRec := doStartTOTPEnrollmentRequest(t, adminapi.StartTOTPEnrollment(db, db), cookie, "password123")
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	var startResp map[string]string
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	secret := startResp["secret"]
	if secret == "" || startResp["otpauth_uri"] == "" {
		t.Fatalf("start response = %v, want nonempty secret and otpauth_uri", startResp)
	}

	userID, ok := currentUserIDByUsername(t, db, "admin")
	if !ok {
		t.Fatal("could not resolve admin's id")
	}

	code, err := totp.GenerateCode(secret, totp.CurrentStep(time.Now()))
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}

	confirmRec := doConfirmTOTPEnrollmentRequest(t, adminapi.ConfirmTOTPEnrollment(db, db), cookie, code)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, want %d, body=%s", confirmRec.Code, http.StatusOK, confirmRec.Body.String())
	}
	var confirmResp map[string][]string
	if err := json.Unmarshal(confirmRec.Body.Bytes(), &confirmResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(confirmResp["recovery_codes"]) != 10 {
		t.Fatalf("recovery_codes = %v, want 10 entries", confirmResp["recovery_codes"])
	}

	reg, err := storage.GetUserTOTP(db, userID)
	if err != nil {
		t.Fatalf("GetUserTOTP: %v", err)
	}
	if reg.ConfirmedAt == nil {
		t.Fatal("ConfirmedAt should be set after confirm")
	}
}

func TestTOTPEnroll_UnconfirmedSecret_LoginDoesNotRequireMFA(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)

	rec := doStartTOTPEnrollmentRequest(t, adminapi.StartTOTPEnrollment(db, db), cookie, "password123")
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	if loginRec.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d, body=%s (unconfirmed enrollment must not require 2FA)", loginRec.Code, http.StatusNoContent, loginRec.Body.String())
	}
}

func TestTOTPEnroll_ConfirmedSecret_LoginRequiresMFA(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)

	startRec := doStartTOTPEnrollmentRequest(t, adminapi.StartTOTPEnrollment(db, db), cookie, "password123")
	var startResp map[string]string
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	code, err := totp.GenerateCode(startResp["secret"], totp.CurrentStep(time.Now()))
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if rec := doConfirmTOTPEnrollmentRequest(t, adminapi.ConfirmTOTPEnrollment(db, db), cookie, code); rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	loginRec := doLoginRequest(t, adminapi.Login(db, db, testBaseURL), "admin", "password123")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d, body=%s (confirmed enrollment must require 2FA)", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}
	var loginResp map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loginResp["mfa_required"] != true {
		t.Fatalf("loginResp = %v, want mfa_required=true", loginResp)
	}
}

func TestStartTOTPEnrollment_WrongPassword_Returns401(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)

	rec := doStartTOTPEnrollmentRequest(t, adminapi.StartTOTPEnrollment(db, db), cookie, "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestStartTOTPEnrollment_AlreadyConfirmed_Returns409(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	userID, _ := currentUserIDByUsername(t, db, "admin")
	enrollConfirmedTOTP(t, db, userID)

	rec := doStartTOTPEnrollmentRequest(t, adminapi.StartTOTPEnrollment(db, db), cookie, "password123")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestTOTPEnroll_DisableThenReEnroll_ExactlyTenRecoveryCodesRemain(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	userID, _ := currentUserIDByUsername(t, db, "admin")
	enrollConfirmedTOTP(t, db, userID)
	if err := storage.InsertRecoveryCodes(db, userID, []string{"leftover-hash"}); err != nil {
		t.Fatalf("InsertRecoveryCodes: %v", err)
	}

	if rec := doDisableTOTPRequest(t, adminapi.DisableTOTP(db, db), cookie, "password123"); rec.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	startRec := doStartTOTPEnrollmentRequest(t, adminapi.StartTOTPEnrollment(db, db), cookie, "password123")
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	var startResp map[string]string
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	code, err := totp.GenerateCode(startResp["secret"], totp.CurrentStep(time.Now()))
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if rec := doConfirmTOTPEnrollmentRequest(t, adminapi.ConfirmTOTPEnrollment(db, db), cookie, code); rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	hashes, err := storage.UnusedRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("UnusedRecoveryCodeHashes: %v", err)
	}
	if len(hashes) != 10 {
		t.Fatalf("unused recovery codes = %d, want exactly 10 (no leftover from the disabled enrollment)", len(hashes))
	}
}

func TestConfirmTOTPEnrollment_WrongCode_Returns401(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)

	if rec := doStartTOTPEnrollmentRequest(t, adminapi.StartTOTPEnrollment(db, db), cookie, "password123"); rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rec := doConfirmTOTPEnrollmentRequest(t, adminapi.ConfirmTOTPEnrollment(db, db), cookie, "000000")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestConfirmTOTPEnrollment_NoPendingEnrollment_Returns404(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)

	rec := doConfirmTOTPEnrollmentRequest(t, adminapi.ConfirmTOTPEnrollment(db, db), cookie, "000000")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestDisableTOTP_CorrectPassword_ClearsRegistrationAndRecoveryCodes(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	userID, _ := currentUserIDByUsername(t, db, "admin")
	enrollConfirmedTOTP(t, db, userID)
	if err := storage.InsertRecoveryCodes(db, userID, []string{"some-hash"}); err != nil {
		t.Fatalf("InsertRecoveryCodes: %v", err)
	}

	rec := doDisableTOTPRequest(t, adminapi.DisableTOTP(db, db), cookie, "password123")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if _, err := storage.GetUserTOTP(db, userID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetUserTOTP after disable: err = %v, want ErrNotFound", err)
	}
	hashes, err := storage.UnusedRecoveryCodeHashes(db, userID)
	if err != nil {
		t.Fatalf("UnusedRecoveryCodeHashes: %v", err)
	}
	if len(hashes) != 0 {
		t.Fatalf("unused recovery codes after disable = %v, want none", hashes)
	}
}

func TestDisableTOTP_WrongPassword_Returns401(t *testing.T) {
	db := newTestDB(t)
	cookie := loginCookie(t, db)
	userID, _ := currentUserIDByUsername(t, db, "admin")
	enrollConfirmedTOTP(t, db, userID)

	rec := doDisableTOTPRequest(t, adminapi.DisableTOTP(db, db), cookie, "wrong-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	if _, err := storage.GetUserTOTP(db, userID); err != nil {
		t.Fatalf("GetUserTOTP after failed disable: err = %v, want registration still present", err)
	}
}
