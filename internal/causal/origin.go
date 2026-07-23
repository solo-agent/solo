package causal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
)

const (
	RunHeader       = "X-Solo-Origin-Run-ID"
	SignatureHeader = "X-Solo-Origin-Signature"
)

func SharedSecret() string {
	if secret := os.Getenv("INTERNAL_TOKEN_SECRET"); secret != "" {
		return secret
	}
	return os.Getenv("JWT_SECRET")
}

func Sign(secret, runID, actorID, channelID string) string {
	if secret == "" || runID == "" || actorID == "" || channelID == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(runID + "\n" + actorID + "\n" + channelID))
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(secret, runID, actorID, channelID, signature string) bool {
	expected := Sign(secret, runID, actorID, channelID)
	if expected == "" || signature == "" {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(signature))
}
