package totp

import (
	"strings"
	"testing"
)

// TestHOTP_RFC6238OfficialTestVectors verifies the shared HOTP core against
// RFC 6238 Appendix B's SHA-1 test vectors: 20-byte ASCII key
// "12345678901234567890", 8-digit truncation, period=30.
func TestHOTP_RFC6238OfficialTestVectors(t *testing.T) {
	key := []byte("12345678901234567890")

	cases := []struct {
		unixTime uint64
		want     string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}

	for _, c := range cases {
		counter := c.unixTime / period
		got := hotp(key, counter, 8)
		if got != c.want {
			t.Errorf("hotp(unixTime=%d, counter=%d) = %q, want %q", c.unixTime, counter, got, c.want)
		}
	}
}

func TestGenerateSecret_ReturnsDecodableUnpaddedBase32(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if secret == "" {
		t.Fatal("secret should not be empty")
	}
	if strings.HasSuffix(secret, "=") {
		t.Errorf("secret %q should be unpadded base32", secret)
	}

	key, err := decodeSecret(secret)
	if err != nil {
		t.Fatalf("decodeSecret: %v", err)
	}
	if len(key) != secretBytes {
		t.Errorf("decoded key length = %d, want %d", len(key), secretBytes)
	}
}

func TestGenerateSecret_ReturnsDistinctSecrets(t *testing.T) {
	a, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	b, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if a == b {
		t.Error("two GenerateSecret calls returned the same secret")
	}
}

func TestValidate_CurrentStepMatches(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	key, err := decodeSecret(secret)
	if err != nil {
		t.Fatalf("decodeSecret: %v", err)
	}

	const atStep = 12345
	code := hotp(key, uint64(atStep), digits)

	step, ok := Validate(secret, code, atStep)
	if !ok {
		t.Fatal("Validate returned ok=false for a code generated at atStep")
	}
	if step != atStep {
		t.Errorf("step = %d, want %d", step, atStep)
	}
}

func TestValidate_OneStepEitherSideMatches(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	key, err := decodeSecret(secret)
	if err != nil {
		t.Fatalf("decodeSecret: %v", err)
	}

	const atStep = 500

	prevCode := hotp(key, atStep-1, digits)
	if step, ok := Validate(secret, prevCode, atStep); !ok || step != atStep-1 {
		t.Errorf("Validate(prevCode) = (%d, %v), want (%d, true)", step, ok, atStep-1)
	}

	nextCode := hotp(key, atStep+1, digits)
	if step, ok := Validate(secret, nextCode, atStep); !ok || step != atStep+1 {
		t.Errorf("Validate(nextCode) = (%d, %v), want (%d, true)", step, ok, atStep+1)
	}
}

func TestValidate_TwoStepsAwayIsRejected(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	key, err := decodeSecret(secret)
	if err != nil {
		t.Fatalf("decodeSecret: %v", err)
	}

	const atStep = 500
	farCode := hotp(key, atStep+2, digits)

	if _, ok := Validate(secret, farCode, atStep); ok {
		t.Error("Validate should reject a code 2 steps away from atStep")
	}
}

func TestValidate_WrongSecretIsRejected(t *testing.T) {
	secretA, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	secretB, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	keyB, err := decodeSecret(secretB)
	if err != nil {
		t.Fatalf("decodeSecret: %v", err)
	}

	code := hotp(keyB, 500, digits)
	if _, ok := Validate(secretA, code, 500); ok {
		t.Error("Validate should reject a code generated with a different secret")
	}
}

func TestValidate_MalformedSecretIsRejected(t *testing.T) {
	if _, ok := Validate("not-valid-base32!!!", "123456", 500); ok {
		t.Error("Validate should reject a malformed secret rather than panic or match")
	}
}

func TestOTPAuthURI_ContainsExpectedFields(t *testing.T) {
	uri := OTPAuthURI("JBSWY3DPEHPK3PXP", "admin", "XordPress")

	wantPrefix := "otpauth://totp/XordPress:admin?"
	if !strings.HasPrefix(uri, wantPrefix) {
		t.Errorf("URI = %q, want prefix %q", uri, wantPrefix)
	}
	for _, want := range []string{"secret=JBSWY3DPEHPK3PXP", "issuer=XordPress", "digits=6", "period=30"} {
		if !strings.Contains(uri, want) {
			t.Errorf("URI %q missing expected substring %q", uri, want)
		}
	}
}
