package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/solo-ai/solo/pkg/config"
)

func TestValidateProductionConfig(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://solo.example.com")
	t.Setenv("DATABASE_URL", "postgres://solo:secret@postgres/solo")
	t.Setenv("ATTACHMENTS_DIR", t.TempDir())
	t.Setenv("ARTIFACTS_DIR", t.TempDir())
	valid := &config.Config{
		JWTSecret:         "0123456789abcdef0123456789abcdef",
		DBURL:             "postgres://solo:secret@postgres/solo",
		PublicURL:         "https://solo.example.com",
		AuthMailTransport: "smtp",
		SMTPHost:          "smtp.example.com",
		SMTPFrom:          "Solo <noreply@example.com>",
		SMTPPort:          "587",
		SMTPTLS:           "starttls",
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
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://solo.example.com")

	tencent := *valid
	tencent.AuthMailTransport = "tencent_ses"
	tencent.TencentCloudSecretID = "secret-id"
	tencent.TencentCloudSecretKey = "secret-key"
	tencent.TencentSESRegion = "ap-guangzhou"
	tencent.TencentSESFrom = "Solo <noreply@solo.example.com>"
	tencent.TencentSESTemplateID = 123
	if err := validateProductionConfig(&tencent); err != nil {
		t.Fatalf("valid Tencent SES config rejected: %v", err)
	}
	tencent.TencentSESTemplateID = 0
	if err := validateProductionConfig(&tencent); err == nil {
		t.Fatal("missing Tencent SES template accepted")
	}
}

func TestValidateStorageDirectories(t *testing.T) {
	root := t.TempDir()
	attachments := filepath.Join(root, "attachments")
	artifacts := filepath.Join(root, "artifacts")
	t.Setenv("ATTACHMENTS_DIR", attachments)
	t.Setenv("ARTIFACTS_DIR", artifacts)
	if err := validateStorageDirectories(); err != nil {
		t.Fatalf("writable storage rejected: %v", err)
	}
	for _, dir := range []string{attachments, artifacts} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("storage directory %q was not created", dir)
		}
	}

	notDirectory := filepath.Join(root, "file")
	if err := os.WriteFile(notDirectory, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATTACHMENTS_DIR", notDirectory)
	if err := validateStorageDirectories(); err == nil {
		t.Fatal("non-directory storage path accepted")
	}
}
