package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/solo-ai/solo/pkg/config"
)

func TestAuthEmailValidationAndSignupPolicy(t *testing.T) {
	email, err := normalizeEmail("  Person@Example.COM ")
	if err != nil || email != "person@example.com" {
		t.Fatalf("normalizeEmail() = %q, %v", email, err)
	}
	if _, err := normalizeEmail("Person <person@example.com>"); err == nil {
		t.Fatal("display-name email must be rejected")
	}
	if err := validatePassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := validatePassword(strings.Repeat("x", 73)); err == nil {
		t.Fatal("bcrypt-truncated password accepted")
	}

	h := &AuthHandler{cfg: &config.Config{AllowSignup: false, AllowedEmailDomains: []string{"example.com"}}}
	if !h.signupAllowed("person@example.com") {
		t.Fatal("allowed domain rejected")
	}
	if h.signupAllowed("person@other.test") {
		t.Fatal("unlisted domain accepted")
	}
}

func TestDevelopmentVerificationCode(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("SOLO_DEV_AUTH_CODE", "314159")
	code, err := verificationCode()
	if err != nil || code != "314159" {
		t.Fatalf("verificationCode() = %q, %v", code, err)
	}
	t.Setenv("APP_ENV", "production")
	code, err = verificationCode()
	if err != nil || len(code) != 6 {
		t.Fatalf("production verificationCode() = %q, %v", code, err)
	}
}

func TestLocalRegistrationAutoVerifyBoundary(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SOLO_DEV_AUTO_VERIFY_LOCAL", "true")

	local := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/v1/auth/register", nil)
	local.RemoteAddr = "127.0.0.1:49152"
	local.Header.Set("Origin", "http://localhost:3000")
	if !localRegistrationAutoVerify(local) {
		t.Fatal("direct localhost registration did not enable automatic verification")
	}

	for name, mutate := range map[string]func(*http.Request){
		"public host":        func(r *http.Request) { r.Host = "solo.example.com" },
		"public origin":      func(r *http.Request) { r.Header.Set("Origin", "https://solo.example.com") },
		"public peer":        func(r *http.Request) { r.RemoteAddr = "203.0.113.8:49152" },
		"forwarded client":   func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113.8") },
		"forwarded host":     func(r *http.Request) { r.Header.Set("X-Forwarded-Host", "solo.example.com") },
		"standard forwarded": func(r *http.Request) { r.Header.Set("Forwarded", "for=127.0.0.1;host=localhost") },
	} {
		t.Run(name, func(t *testing.T) {
			r := local.Clone(local.Context())
			r.Header = local.Header.Clone()
			mutate(r)
			if localRegistrationAutoVerify(r) {
				t.Fatal("non-local registration enabled automatic verification")
			}
		})
	}

	t.Run("production", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		if localRegistrationAutoVerify(local) {
			t.Fatal("production registration enabled automatic verification")
		}
	})
}

func TestEmailCodeHashIsBoundToAccountPurposeAndSecret(t *testing.T) {
	base := codeHash("secret-a", "person@example.com", challengeRegister, "123456")
	for name, changed := range map[string]string{
		"secret":  codeHash("secret-b", "person@example.com", challengeRegister, "123456"),
		"email":   codeHash("secret-a", "other@example.com", challengeRegister, "123456"),
		"purpose": codeHash("secret-a", "person@example.com", challengePasswordReset, "123456"),
		"code":    codeHash("secret-a", "person@example.com", challengeRegister, "654321"),
	} {
		if changed == base {
			t.Fatalf("hash was not bound to %s", name)
		}
	}
}
