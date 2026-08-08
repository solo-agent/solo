package main

import (
	"testing"

	"github.com/solo-ai/solo/pkg/config"
)

func TestValidateProductionConfig(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://solo.example.com")
	t.Setenv("DATABASE_URL", "postgres://solo:secret@postgres/solo")
	valid := &config.Config{
		JWTSecret: "0123456789abcdef0123456789abcdef",
		DBURL:     "postgres://solo:secret@postgres/solo",
		PublicURL: "https://solo.example.com",
		SMTPHost:  "smtp.example.com",
		SMTPFrom:  "Solo <noreply@example.com>",
		SMTPPort:  "587",
		SMTPTLS:   "starttls",
	}
	if err := validateProductionConfig(valid); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}

	weak := *valid
	weak.JWTSecret = "change-me"
	if err := validateProductionConfig(&weak); err == nil {
		t.Fatal("weak JWT secret accepted")
	}

	t.Setenv("DATABASE_URL", "")
	if err := validateProductionConfig(valid); err == nil {
		t.Fatal("missing DATABASE_URL accepted")
	}
	t.Setenv("DATABASE_URL", valid.DBURL)

	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	if err := validateProductionConfig(valid); err == nil {
		t.Fatal("wildcard credentialed CORS accepted")
	}
}
