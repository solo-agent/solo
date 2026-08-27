package service

import "testing"

func TestLarkSecretRoundTrip(t *testing.T) {
	const secret = "app-secret-value"
	encrypted, err := encryptLarkSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := decryptLarkSecret(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != secret {
		t.Fatalf("got %q, want %q", decrypted, secret)
	}
}
