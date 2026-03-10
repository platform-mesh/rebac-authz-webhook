package retry

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Tracker tracks whether something should be retried.
type Tracker interface {
	ShouldRetry(key any) bool
	Retried(key any)
}

// ExpiringRetryTracker tracks how often something for a given key was tried and
// stops tracking when a given maximum or TTL was reached.
type ExpiringRetryTracker struct {
	keys map[any]*count
	max  uint
	ttl  time.Duration

	m sync.Mutex
}

type count struct {
	c uint
	t time.Time
}

func (c *count) Add() {
	c.c += 1
}

func (c *count) expired(ttl time.Duration) bool {
	return time.Since(c.t) > ttl
}

// NewExpiringRetryTracker returns a Tracker that tracks of to max
// per key, resetting the count when no count has occurred for ttl.
func NewExpiringRetryTracker(max uint, ttl time.Duration) *ExpiringRetryTracker {
	return &ExpiringRetryTracker{
		keys: make(map[any]*count),
		max:  max,
		ttl:  ttl,
	}
}

// count returns the current retry count for key, or 0 if the count has
// expired. Expired entries are removed. Caller must hold t.m.
func (t *ExpiringRetryTracker) count(key any) uint {
	c, ok := t.keys[key]

	if !ok {
		return 0
	}

	if c.expired(t.ttl) {
		klog.V(5).InfoS("Retry count expired, resetting", "key", key, "previousCount", c.c, "ttl", t.ttl)
		delete(t.keys, key)
		return 0
	}

	return c.c
}

// PeriodicCleanup checks every key for expiration in a given interval. Blocks
// until ctx is cancelled.
func (t *ExpiringRetryTracker) PeriodicCleanup(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			klog.V(5).InfoS("Running periodic cleanup")
			t.cleanup()
		}
	}
}

func (t *ExpiringRetryTracker) cleanup() {
	t.m.Lock()
	defer t.m.Unlock()

	for key, count := range t.keys {
		if count.expired(t.ttl) {
			delete(t.keys, key)
		}
	}
}

// ShouldRetry reports whether the key should be retried, i.e. the maximum retries have not been reached.
func (t *ExpiringRetryTracker) ShouldRetry(key any) bool {
	t.m.Lock()
	defer t.m.Unlock()

	c := t.count(key)
	should := c < t.max
	klog.V(5).InfoS("Should retry", "key", key, "count", c, "max", t.max, "should", should)

	return should
}

// Retried records that the key was tried.
func (t *ExpiringRetryTracker) Retried(key any) {
	t.m.Lock()
	defer t.m.Unlock()

	now := time.Now()
	_, ok := t.keys[key]
	if !ok {
		t.keys[key] = &count{t: now}
	}
	t.keys[key].Add()

	klog.V(5).InfoS("Recorded retry", "key", key, "count", t.keys[key].c, "max", t.max)
}

var _ Tracker = &ExpiringRetryTracker{}
