package handler

import (
	"strings"
	"testing"
)

func TestOnboardingUniqueName(t *testing.T) {
	got := onboardingUniqueName("welcome-card-verify", "12345678-aaaa-bbbb-cccc-123456789abc")
	if got != "welcome-card-verify-12345678" {
		t.Fatalf("unexpected unique onboarding name: %q", got)
	}
	if len([]rune(onboardingUniqueName(strings.Repeat("a", 120), "12345678-aaaa"))) > 100 {
		t.Fatal("unique onboarding name exceeded channel name limit")
	}
}
