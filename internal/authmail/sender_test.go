package authmail

import (
	"encoding/json"
	"testing"

	"github.com/solo-ai/solo/pkg/config"
)

func TestNewTencentSESRequest(t *testing.T) {
	cfg := &config.Config{
		TencentSESFrom:       "Solo <noreply@soloagent.team>",
		TencentSESTemplateID: 42,
	}
	request, err := newTencentSESRequest(cfg, "user@example.com", "123456", "password_reset")
	if err != nil {
		t.Fatal(err)
	}
	if got := *request.Subject; got != "Reset your Solo password" {
		t.Fatalf("subject = %q", got)
	}
	if got := *request.FromEmailAddress; got != cfg.TencentSESFrom {
		t.Fatalf("from = %q", got)
	}
	if len(request.Destination) != 1 || *request.Destination[0] != "user@example.com" {
		t.Fatalf("destination = %#v", request.Destination)
	}
	if got := *request.Template.TemplateID; got != 42 {
		t.Fatalf("template ID = %d", got)
	}
	if got := *request.TriggerType; got != 1 {
		t.Fatalf("trigger type = %d", got)
	}
	var data map[string]string
	if err := json.Unmarshal([]byte(*request.Template.TemplateData), &data); err != nil {
		t.Fatal(err)
	}
	if data["code"] != "123456" || data["intro"] != "Enter this code to reset your Solo password." {
		t.Fatalf("template data = %#v", data)
	}
	if _, err := newTencentSESRequest(cfg, "User <user@example.com>", "123456", "register"); err == nil {
		t.Fatal("display-name recipient accepted")
	}
}
