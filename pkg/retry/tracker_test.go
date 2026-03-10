package retry

import (
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
