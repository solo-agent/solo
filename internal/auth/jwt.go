package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultJWTSecret       = "solo-dev-secret-change-in-production"
	AccessTokenDuration       = 15 * time.Minute
	AgentAccessTokenDuration  = 365 * 24 * time.Hour // effectively permanent: refreshed per session
	RefreshTokenDuration      = 7 * 24 * time.Hour
)

var (
	jwtSecret     []byte
	jwtSecretOnce sync.Once
)

// JWTSecret returns the JWT signing key, loaded from JWT_SECRET env var.
func JWTSecret() []byte {
	jwtSecretOnce.Do(func() {
		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = defaultJWTSecret
		}
		jwtSecret = []byte(secret)
	})
	return jwtSecret
}

// SoloClaims represents the JWT claims for Solo.
type SoloClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
	Name  string `json:"name"`
}

// GenerateAgentToken creates a long-lived JWT access token for agent sessions (24h).
func GenerateAgentToken(agentID, displayName string) (string, error) {
	return generateToken(agentID, agentID+"@solo.agent", displayName, AgentAccessTokenDuration)
}

// GenerateAccessToken creates a short-lived JWT access token (15 min).
func GenerateAccessToken(userID, email, displayName string) (string, error) {
	return generateToken(userID, email, displayName, AccessTokenDuration)
}

func generateToken(userID, email, displayName string, duration time.Duration) (string, error) {
	now := time.Now()
	claims := SoloClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			Issuer:    "solo",
		},
		Email: email,
		Name:  displayName,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JWTSecret())
}

// GenerateRefreshToken creates a refresh token (random 32-byte hex).
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ValidateToken parses and validates a JWT token string, returning the claims.
func ValidateToken(tokenString string) (*SoloClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &SoloClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return JWTSecret(), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*SoloClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// HashToken returns the hex-encoded SHA-256 hash of a token string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
