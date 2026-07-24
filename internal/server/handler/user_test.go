package handler

import "testing"

func TestUserAvatarValueValidation(t *testing.T) {
	if !isUserAvatarPreset("dicebear:pixel-art:11") {
		t.Fatal("expected the last avatar preset to be valid")
	}
	for _, value := range []string{
		"dicebear:pixel-art:12",
		"dicebear:bottts:0",
		"https://example.com/avatar.png",
	} {
		if isUserAvatarPreset(value) {
			t.Fatalf("unexpected valid preset: %q", value)
		}
	}

	id := "4a599473-34a9-4b34-a6f6-d855b05c3d03"
	for _, value := range []string{
		"/api/v1/attachments/" + id,
		"/api/v1/attachments/" + id + "/thumbnail",
	} {
		if got, ok := userAvatarAttachmentID(value); !ok || got != id {
			t.Fatalf("userAvatarAttachmentID(%q) = %q, %v", value, got, ok)
		}
	}
	if _, ok := userAvatarAttachmentID("/api/v1/attachments/" + id + "/download"); ok {
		t.Fatal("unexpected non-avatar attachment URL accepted")
	}
}
