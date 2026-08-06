package service

import (
	"testing"
	"time"
)

func TestAgentSendRateIsIsolatedAndRecovers(t *testing.T) {
	svc := &AgentService{
		agentSendAttempts: make(map[string][]time.Time),
		agentSendLimit:    2,
		agentSendWindow:   10 * time.Second,
	}
	for i := 0; i < 2; i++ {
		if allowed, _ := svc.CheckAgentSendRate("agent-a"); !allowed {
			t.Fatalf("agent-a attempt %d was rejected", i+1)
		}
	}
	if allowed, retryAfter := svc.CheckAgentSendRate("agent-a"); allowed || retryAfter <= 0 {
		t.Fatalf("agent-a third attempt = allowed %v retry %v", allowed, retryAfter)
	}
	if allowed, _ := svc.CheckAgentSendRate("agent-b"); !allowed {
		t.Fatal("agent-b was affected by agent-a")
	}

	svc.agentSendAttempts["agent-a"] = []time.Time{time.Now().Add(-11 * time.Second)}
	if allowed, _ := svc.CheckAgentSendRate("agent-a"); !allowed {
		t.Fatal("agent-a did not recover after the window")
	}
}

func TestAgentCascadeStopsAtThresholdAndRecovers(t *testing.T) {
	svc := &AgentService{
		cascadeWindow:    10 * time.Second,
		cascadeThreshold: 3,
		cascadeCooldown:  time.Minute,
	}
	if !svc.allowAgentCascade("channel-a") || !svc.allowAgentCascade("channel-a") {
		t.Fatal("cascade stopped before its threshold")
	}
	if svc.allowAgentCascade("channel-a") || svc.allowAgentCascade("channel-a") {
		t.Fatal("cascade continued during cooldown")
	}
	if !svc.allowAgentCascade("channel-b") {
		t.Fatal("channel-b was affected by channel-a")
	}

	value, _ := svc.cascadeMap.Load("channel-a")
	count := value.(*cascadeCount)
	count.cooldownUntil = time.Now().Add(-time.Second)
	count.windowStart = time.Now().Add(-11 * time.Second)
	if !svc.allowAgentCascade("channel-a") {
		t.Fatal("channel-a did not recover after cooldown")
	}
}
