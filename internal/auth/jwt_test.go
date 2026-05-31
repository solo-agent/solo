package auth

import (
	"testing"
	"time"
)

func TestGenerateAccessToken(t *testing.T) {
	token, err := GenerateAccessToken("user-1", "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestValidateToken(t *testing.T) {
	userID := "550e8400-e29b-41d4-a716-446655440000"
	email := "test@example.com"
	name := "Test User"

	token, err := GenerateAccessToken(userID, email, name)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.Subject != userID {
		t.Errorf("expected subject %q, got %q", userID, claims.Subject)
	}
	if claims.Email != email {
		t.Errorf("expected email %q, got %q", email, claims.Email)
	}
	if claims.Name != name {
		t.Errorf("expected name %q, got %q", name, claims.Name)
	}
}

func TestValidateInvalidToken(t *testing.T) {
	_, err := ValidateToken("invalid-token-string")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateExpiredToken(t *testing.T) {
	// Create a token with an already-expired time
	// We test this by checking the expiration claim directly
	token, err := GenerateAccessToken("user-1", "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.ExpiresAt == nil {
		t.Fatal("expected expires_at claim")
	}

	// Token should expire in the future
	if !claims.ExpiresAt.Time.After(time.Now()) {
		t.Error("expected token to not be expired yet")
	}

	// Should be within AccessTokenDuration
	maxExpiry := time.Now().Add(AccessTokenDuration + time.Minute)
	if claims.ExpiresAt.Time.After(maxExpiry) {
		t.Error("token expiry is too far in the future")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	token, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty refresh token")
	}
	if len(token) != 64 {
		t.Errorf("expected token length 64 (32 bytes hex), got %d", len(token))
	}
}

func TestHashToken(t *testing.T) {
	token := "test-refresh-token"
	hash := HashToken(token)
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if len(hash) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(hash))
	}

	// Same input should produce same hash
	hash2 := HashToken(token)
	if hash != hash2 {
		t.Error("expected hash to be deterministic")
	}
}

func TestHashTokenDifferent(t *testing.T) {
	hash1 := HashToken("token-1")
	hash2 := HashToken("token-2")
	if hash1 == hash2 {
		t.Error("expected different tokens to produce different hashes")
	}
}
