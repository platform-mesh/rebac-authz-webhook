package retry

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCountingTracker_ShouldRetry_AllowsRetriesUnderMax(t *testing.T) {
	tracker := NewExpiringRetryTracker(3, time.Hour)
	key := "test-key"

	for i := 0; i < 3; i++ {
		if !tracker.ShouldRetry(key) {
			t.Errorf("ShouldRetry(key) = false at retry %d, want true", i)
		}
		tracker.Retried(key)
	}

	if tracker.ShouldRetry(key) {
		t.Error("ShouldRetry(key) = true after max retries, want false")
	}
}

func TestCountingTracker_ShouldRetry_ReturnsTrueForNewKey(t *testing.T) {
	tracker := NewExpiringRetryTracker(1, time.Hour)

	if !tracker.ShouldRetry("new-key") {
		t.Error("ShouldRetry(new-key) = false, want true")
	}
}

func TestCountingTracker_KeysTrackedIndependently(t *testing.T) {
	tracker := NewExpiringRetryTracker(1, time.Hour)

	tracker.Retried("key-a")
	tracker.Retried("key-b")

	if tracker.ShouldRetry("key-a") {
		t.Error("ShouldRetry(key-a) = true after max, want false")
	}
	if tracker.ShouldRetry("key-b") {
		t.Error("ShouldRetry(key-b) = true after max, want false")
	}
	if !tracker.ShouldRetry("key-c") {
		t.Error("ShouldRetry(key-c) = false for new key, want true")
	}
}

func TestCountingTracker_CountResetsAfterTTL(t *testing.T) {
	ttl := 50 * time.Millisecond
	tracker := NewExpiringRetryTracker(2, ttl)
	key := "ttl-key"

	tracker.Retried(key)
	tracker.Retried(key)
	if tracker.ShouldRetry(key) {
		t.Error("ShouldRetry(key) = true at max, want false")
	}

	time.Sleep(ttl + 10*time.Millisecond)

	if !tracker.ShouldRetry(key) {
		t.Error("ShouldRetry(key) = false after TTL, want true")
	}
	tracker.Retried(key)
	if !tracker.ShouldRetry(key) {
		t.Error("ShouldRetry(key) = false after one retry post-reset, want true")
	}
	tracker.Retried(key)
	if tracker.ShouldRetry(key) {
		t.Error("ShouldRetry(key) = true at max after reset, want false")
	}
}

func TestCountingTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewExpiringRetryTracker(10, time.Hour)
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := id
			for j := 0; j < 10; j++ {
				tracker.ShouldRetry(key)
				tracker.Retried(key)
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		if tracker.ShouldRetry(i) {
			t.Errorf("ShouldRetry(%d) = true after 10 retries with max 10, want false", i)
		}
	}
}

func TestNewCountingTracker_InitializesMap(t *testing.T) {
	tracker := NewExpiringRetryTracker(1, time.Hour)
	if tracker == nil {
		t.Fatal("NewCountingTracker returned nil")
	}
	if !tracker.ShouldRetry("any") {
		t.Error("new tracker ShouldRetry = false, want true")
	}
}

func TestExpiringRetryTracker_PeriodicCleanup_DeletesExpiredElements(t *testing.T) {
	ttl := 30 * time.Millisecond
	tracker := NewExpiringRetryTracker(10, ttl)

	tracker.Retried("key-1")
	tracker.Retried("key-2")
	tracker.Retried("key-3")
	if len(tracker.keys) != 3 {
		t.Errorf("before expiry: len(keys) = %d, want 3", len(tracker.keys))
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tracker.PeriodicCleanup(ctx, 5*time.Millisecond)
		close(done)
	}()

	time.Sleep(ttl * 2)
	cancel()
	<-done

	if len(tracker.keys) != 0 {
		t.Errorf("after PeriodicCleanup: len(keys) = %d, want 0 (expired elements should be deleted)", len(tracker.keys))
	}
}
