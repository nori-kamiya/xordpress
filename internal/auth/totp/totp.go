// Package totp implements RFC 6238 TOTP (and its RFC 4226 HOTP core) using
// only the standard library, for XordPress's 2FA feature (design.md §6.1,
// §7). Secrets are stored and exchanged as unpadded base32, matching the
// otpauth:// URI convention every TOTP app (Google Authenticator, Authy,
// ...) expects.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// period is the TOTP time step in seconds (RFC 6238 §4's default).
	period = 30

	// digits is the code length. RFC 6238's own test vectors use 8 digits,
	// but every mainstream TOTP app (Google Authenticator, Authy, ...)
	// hard-codes 6, so 6 is what this package's public API produces and
	// validates — RFC compliance is verified against the core HOTP
	// function directly (see totp_test.go), independent of this choice.
	digits = 6

	// secretBytes is the raw secret length in bytes (160 bits), RFC 4226
	// §4's recommended HMAC-SHA1 key size.
	secretBytes = 20
)

// GenerateSecret returns a new random secret, base32-encoded without
// padding for direct use in an otpauth:// URI.
func GenerateSecret() (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("totp: generate secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// GenerateCode returns the TOTP code for secret at atStep, the inverse of
// Validate. Server-side code generation isn't otherwise needed by the login
// flow (only the authenticator app generates codes there), but it's the
// only way to produce a real, secret-derived code for tests that exercise
// Validate's success path without hand-rolling RFC 4226's HOTP algorithm a
// second time.
func GenerateCode(secret string, atStep int64) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", fmt.Errorf("totp: generate code: %w", err)
	}
	return hotp(key, uint64(atStep), digits), nil
}

// Validate reports whether code is a valid TOTP code for secret at atStep
// or within ±1 step of it (RFC 6238 §5.2's recommended clock-skew
// tolerance). On success it returns the exact step that matched, so
// callers can enforce replay protection by rejecting any step at or below
// the last one they accepted (design.md §6.1 step 3). Each candidate is
// compared in constant time to avoid leaking a timing signal on the
// secret-derived code.
func Validate(secret, code string, atStep int64) (step int64, ok bool) {
	key, err := decodeSecret(secret)
	if err != nil {
		return 0, false
	}

	for _, offset := range [3]int64{0, -1, 1} {
		s := atStep + offset
		if s < 0 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(key, uint64(s), digits)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// CurrentStep returns the TOTP step (floor(unixTime/period)) for t, for
// callers that need to pass atStep to Validate.
func CurrentStep(t time.Time) int64 {
	return t.Unix() / period
}

// OTPAuthURI returns the otpauth://totp/... URI TOTP apps scan to enroll
// secret, per the de facto "Key URI Format" convention (issuer and digits
// query parameters, "Issuer:accountName" label).
func OTPAuthURI(secret, accountName, issuer string) string {
	label := url.PathEscape(fmt.Sprintf("%s:%s", issuer, accountName))
	q := url.Values{
		"secret": {secret},
		"issuer": {issuer},
		"digits": {strconv.Itoa(digits)},
		"period": {strconv.Itoa(period)},
	}
	return fmt.Sprintf("otpauth://totp/%s?%s", label, q.Encode())
}

// decodeSecret decodes a base32 secret, tolerating lowercase input and a
// missing padding suffix (both common when a secret is hand-typed or
// copy-pasted without its trailing '=' characters).
func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	if n := len(s) % 8; n != 0 {
		s += strings.Repeat("=", 8-n)
	}
	return base32.StdEncoding.DecodeString(s)
}

// hotp implements RFC 4226's HOTP(key, counter) with n-digit dynamic
// truncation, the shared core of every TOTP computation (TOTP is HOTP with
// counter = floor(unixTime/period)).
func hotp(key []byte, counter uint64, n int) string {
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(counterBytes[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < n; i++ {
		mod *= 10
	}
	code := truncated % mod
	return fmt.Sprintf("%0*d", n, code)
}
