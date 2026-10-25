package coach

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPendingPutTakeOnce(t *testing.T) {
	p := NewPending(time.Minute, 10)
	id, err := p.Put(Conversation{Question: "Are eggs healthy?"})
	if err != nil {
		t.Fatal(err)
	}

	conv, ok := p.Take(id)
	if !ok || conv.Question != "Are eggs healthy?" {
		t.Fatalf("Take = %+v, %v", conv, ok)
	}
	if _, ok := p.Take(id); ok {
		t.Error("an ID must only work once")
	}
	if _, ok := p.Take("made-up-id"); ok {
		t.Error("unknown ID should not be found")
	}
}

func TestPendingExpires(t *testing.T) {
	p := NewPending(time.Minute, 10)
	clock := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return clock } // a fake clock we control

	id, _ := p.Put(Conversation{Question: "q"})
	clock = clock.Add(2 * time.Minute) // "wait" two minutes, instantly

	if _, ok := p.Take(id); ok {
		t.Error("expired ID should not be found")
	}
}

func TestPendingLimitAndCleanup(t *testing.T) {
	p := NewPending(time.Minute, 2)
	clock := time.Now()
	p.now = func() time.Time { return clock }

	p.Put(Conversation{})
	p.Put(Conversation{})
	if _, err := p.Put(Conversation{}); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("err = %v, want ErrTooManyPending", err)
	}

	clock = clock.Add(2 * time.Minute) // both old entries expire...
	if _, err := p.Put(Conversation{}); err != nil {
		t.Fatalf("after expiry Put should work again: %v", err)
	}
	if p.Len() != 1 {
		t.Errorf("Len = %d, want 1 (expired entries removed)", p.Len())
	}
}

// Run with `go test -race ./...` to let Go check for unsafe concurrent access.
func TestPendingConcurrentUse(t *testing.T) {
	p := NewPending(time.Minute, 1000)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := p.Put(Conversation{Question: "q"})
			if err != nil {
				t.Error(err)
				return
			}
			if _, ok := p.Take(id); !ok {
				t.Error("lost an item")
			}
		}()
	}
	wg.Wait()
	if p.Len() != 0 {
		t.Errorf("Len = %d, want 0", p.Len())
	}
}
