package main

import "testing"

func TestResolveInternalToken(t *testing.T) {
	t.Run("uses dedicated internal token", func(t *testing.T) {
		if got := resolveInternalToken("internal-secret", "jwt-secret"); got != "internal-secret" {
			t.Fatalf("resolveInternalToken() = %q, want internal-secret", got)
		}
	})

	t.Run("falls back to jwt secret for backward compatibility", func(t *testing.T) {
		if got := resolveInternalToken("", "jwt-secret"); got != "jwt-secret" {
			t.Fatalf("resolveInternalToken() = %q, want jwt-secret", got)
		}
	})
}
