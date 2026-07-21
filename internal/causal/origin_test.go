package causal

import "testing"

func TestOriginSignatureBindsRunActorAndChannel(t *testing.T) {
	signature := Sign("secret", "run-1", "agent-1", "channel-1")
	if !Verify("secret", "run-1", "agent-1", "channel-1", signature) {
		t.Fatal("valid origin signature was rejected")
	}
	for _, altered := range []struct{ run, actor, channel string }{
		{"run-2", "agent-1", "channel-1"},
		{"run-1", "agent-2", "channel-1"},
		{"run-1", "agent-1", "channel-2"},
	} {
		if Verify("secret", altered.run, altered.actor, altered.channel, signature) {
			t.Fatalf("altered origin was accepted: %+v", altered)
		}
	}
}

func TestOriginSignatureRejectsMissingSecretOrSignature(t *testing.T) {
	if Sign("", "run-1", "agent-1", "channel-1") != "" {
		t.Fatal("empty secret produced a signature")
	}
	if Verify("secret", "run-1", "agent-1", "channel-1", "") {
		t.Fatal("empty signature was accepted")
	}
}
