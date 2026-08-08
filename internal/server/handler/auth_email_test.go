package handler

import (
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
