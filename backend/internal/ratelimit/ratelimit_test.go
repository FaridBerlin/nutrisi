package ratelimit

import (
	"sync"
	"testing"
	"time"
)

// newTestLimiter returns a limiter with a fake clock we can move by hand.
func newTestLimiter(count int, period time.Duration, burst int) (*Limiter, *time.Time) {
	l := New(count, period, burst)
	clock := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return clock }
	return l, &clock
}

func TestBurstThenRefuse(t *testing.T) {
	l, _ := newTestLimiter(30, time.Hour, 3) // 1 token every 2 minutes, bucket of 3

	for i := range 3 {
		if ok, _ := l.Allow("1.2.3.4"); !ok {
			t.Fatalf("request %d should be allowed (burst)", i+1)
		}
	}
	ok, retry := l.Allow("1.2.3.4")
	if ok {
		t.Fatal("4th request should be refused")
	}
	if retry != 2*time.Minute {
		t.Errorf("retryAfter = %s, want 2m (time for one new token)", retry)
	}
}

func TestTokensComeBack(t *testing.T) {
	l, clock := newTestLimiter(30, time.Hour, 1)

	l.Allow("a")
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("bucket should be empty")
	}
	*clock = clock.Add(2 * time.Minute) // one token flows back
	if ok, _ := l.Allow("a"); !ok {
		t.Error("after 2 minutes one request should be allowed again")
	}
}

func TestClientsAreIndependent(t *testing.T) {
	l, _ := newTestLimiter(30, time.Hour, 1)
	l.Allow("alice")
	if ok, _ := l.Allow("bob"); !ok {
		t.Error("bob must not pay for alice's requests")
	}
}

func TestRefusedRequestsDontUseTokens(t *testing.T) {
	l, clock := newTestLimiter(30, time.Hour, 1)
	l.Allow("a")
	for range 5 {
		l.Allow("a") // refused - must not push the next token further away
	}
	*clock = clock.Add(2 * time.Minute)
	if ok, _ := l.Allow("a"); !ok {
		t.Error("refused requests should not have consumed future tokens")
	}
}

func TestIdleClientsAreForgotten(t *testing.T) {
	l, clock := newTestLimiter(30, time.Hour, 5)
	l.Allow("old")
	*clock = clock.Add(2 * time.Hour)
	l.Allow("new") // triggers a sweep
	if l.Len() != 1 {
		t.Errorf("Len = %d, want 1 (idle client removed)", l.Len())
	}
}

// Run with -race: many goroutines using one limiter at once.
func TestConcurrentUse(t *testing.T) {
	l := New(1000, time.Second, 1000)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Allow("same-ip")
		}()
	}
	wg.Wait()
}
