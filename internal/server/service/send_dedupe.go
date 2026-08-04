package service

import (
	"context"
	"sync"
	"time"
)

// SendDedupe coalesces concurrent message sends and briefly caches successful results.
type SendDedupe struct {
	mu       sync.Mutex
	entries  map[string]*sendDedupeEntry
	capacity int
	ttl      time.Duration
}

type sendDedupeEntry struct {
	done      chan struct{}
	result    *SendDedupeResult
	createdAt time.Time
	expiresAt time.Time
}

type SendDedupeResult struct {
	Status int
	Body   any
}

type SendDedupeClaim struct {
	cache *SendDedupe
	key   string
	entry *sendDedupeEntry
}

func NewSendDedupe(capacity int, ttl time.Duration) *SendDedupe {
	return &SendDedupe{entries: make(map[string]*sendDedupeEntry), capacity: capacity, ttl: ttl}
}

// Acquire returns either ownership of a new send or the completed result for a duplicate.
func (d *SendDedupe) Acquire(ctx context.Context, key string) (*SendDedupeClaim, *SendDedupeResult, error) {
	for {
		now := time.Now()
		d.mu.Lock()
		d.pruneExpired(now)
		if entry, ok := d.entries[key]; ok {
			if entry.result != nil {
				result := *entry.result
				d.mu.Unlock()
				return nil, &result, nil
			}
			done := entry.done
			d.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}

		entry := &sendDedupeEntry{done: make(chan struct{}), createdAt: now}
		d.entries[key] = entry
		d.mu.Unlock()
		return &SendDedupeClaim{cache: d, key: key, entry: entry}, nil, nil
	}
}

func (c *SendDedupeClaim) Complete(status int, body any) {
	if c == nil {
		return
	}
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	entry, ok := c.cache.entries[c.key]
	if !ok || entry != c.entry || entry.result != nil {
		return
	}
	entry.result = &SendDedupeResult{Status: status, Body: body}
	entry.expiresAt = time.Now().Add(c.cache.ttl)
	close(entry.done)
	c.cache.evictCompleted()
}

// Abort releases waiters after a failed send so one of them can retry the work.
func (c *SendDedupeClaim) Abort() {
	if c == nil {
		return
	}
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	entry, ok := c.cache.entries[c.key]
	if !ok || entry != c.entry || entry.result != nil {
		return
	}
	delete(c.cache.entries, c.key)
	close(entry.done)
}

func (d *SendDedupe) pruneExpired(now time.Time) {
	for key, entry := range d.entries {
		if entry.result != nil && !entry.expiresAt.After(now) {
			delete(d.entries, key)
		}
	}
}

func (d *SendDedupe) evictCompleted() {
	for len(d.entries) > d.capacity {
		var oldestKey string
		var oldest time.Time
		for key, entry := range d.entries {
			if entry.result != nil && (oldestKey == "" || entry.createdAt.Before(oldest)) {
				oldestKey, oldest = key, entry.createdAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(d.entries, oldestKey)
	}
}
