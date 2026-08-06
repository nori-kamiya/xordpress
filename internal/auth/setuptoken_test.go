package auth

import (
	"encoding/base64"
	"testing"
)

func TestGenerateSetupToken_ReturnsValidBase64OfSufficientLength(t *testing.T) {
	token, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken returned error: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not valid base64: %v", err)
	}

	if len(decoded) < 32 {
		t.Fatalf("decoded token length = %d, want >= 32", len(decoded))
	}
}

func TestGenerateSetupToken_GeneratesDistinctTokens(t *testing.T) {
	a, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken returned error: %v", err)
	}

	b, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken returned error: %v", err)
	}

	if a == b {
		t.Fatalf("expected two generated tokens to differ, got equal values %q", a)
	}
}

func TestCompareSetupToken_GivenMatchingTokens_ThenReturnsTrue(t *testing.T) {
	token, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken returned error: %v", err)
	}

	if !CompareSetupToken(token, token) {
		t.Fatalf("CompareSetupToken(%q, %q) = false, want true", token, token)
	}
}

func TestCompareSetupToken_GivenMismatchedTokens_ThenReturnsFalse(t *testing.T) {
	if CompareSetupToken("expected-token", "different-token") {
		t.Fatal("CompareSetupToken with mismatched tokens = true, want false")
	}
}

func TestCompareSetupToken_GivenEmptyExpected_ThenAlwaysReturnsFalse(t *testing.T) {
	if CompareSetupToken("", "") {
		t.Fatal("CompareSetupToken(\"\", \"\") = true, want false")
	}

	if CompareSetupToken("", "some-provided-token") {
		t.Fatal("CompareSetupToken(\"\", \"some-provided-token\") = true, want false")
	}
}
