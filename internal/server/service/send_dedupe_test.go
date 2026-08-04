package service

import (
	"context"
	"testing"
	"time"
)

func TestSendDedupeCoalescesConcurrentSends(t *testing.T) {
	cache := NewSendDedupe(1000, time.Minute)
	owner, cached, err := cache.Acquire(context.Background(), "sender:client")
	if err != nil || owner == nil || cached != nil {
		t.Fatalf("first acquire = claim %v, cached %v, err %v", owner, cached, err)
	}

	result := make(chan *SendDedupeResult, 1)
	go func() {
		_, duplicate, acquireErr := cache.Acquire(context.Background(), "sender:client")
		if acquireErr != nil {
			t.Errorf("duplicate acquire: %v", acquireErr)
		}
		result <- duplicate
	}()

	select {
	case <-result:
		t.Fatal("duplicate returned before the owner completed")
	case <-time.After(10 * time.Millisecond):
	}

	owner.Complete(201, "created")
	select {
	case duplicate := <-result:
		if duplicate == nil || duplicate.Status != 201 || duplicate.Body != "created" {
			t.Fatalf("duplicate result = %#v", duplicate)
		}
	case <-time.After(time.Second):
		t.Fatal("duplicate did not receive the completed result")
	}
}

func TestSendDedupeAbortAllowsRetry(t *testing.T) {
	cache := NewSendDedupe(1000, time.Minute)
	owner, _, _ := cache.Acquire(context.Background(), "sender:client")
	owner.Abort()

	retry, cached, err := cache.Acquire(context.Background(), "sender:client")
	if err != nil || retry == nil || cached != nil {
		t.Fatalf("retry acquire = claim %v, cached %v, err %v", retry, cached, err)
	}
	retry.Abort()
}

func TestSendDedupeExpiresAndCapsCompletedResults(t *testing.T) {
	expiring := NewSendDedupe(1, time.Millisecond)
	owner, _, _ := expiring.Acquire(context.Background(), "expired")
	owner.Complete(201, "old")
	time.Sleep(2 * time.Millisecond)
	retry, cached, err := expiring.Acquire(context.Background(), "expired")
	if err != nil || retry == nil || cached != nil {
		t.Fatalf("expired acquire = claim %v, cached %v, err %v", retry, cached, err)
	}
	retry.Abort()

	capped := NewSendDedupe(1, time.Minute)
	first, _, _ := capped.Acquire(context.Background(), "first")
	first.Complete(201, "first")
	second, _, _ := capped.Acquire(context.Background(), "second")
	second.Complete(201, "second")
	replaced, cached, err := capped.Acquire(context.Background(), "first")
	if err != nil || replaced == nil || cached != nil {
		t.Fatalf("evicted acquire = claim %v, cached %v, err %v", replaced, cached, err)
	}
	replaced.Abort()
}
