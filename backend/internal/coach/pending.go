package coach

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"
)

// ErrTooManyPending means the store is full. It protects the server's memory.
var ErrTooManyPending = errors.New("coach: too many pending questions")

// Pending holds questions between two requests:
//
//  1. POST /coach/chat checks the question and stores it here under a random ID.
//  2. The browser opens GET /coach/stream/{id} (EventSource can only do GET,
//     and a URL can't carry a long history), and the stream handler takes the
//     question out again.
//
// Each ID works once and expires after ttl, so old entries can't pile up.
//
// Many requests run at the same time (each in its own goroutine), so the map
// is protected by a mutex: only one goroutine may touch it at a time.
type Pending struct {
	mu    sync.Mutex
	items map[string]pendingItem
	ttl   time.Duration
	max   int
	now   func() time.Time // replaceable in tests, so we don't have to sleep
}

type pendingItem struct {
	conv    Conversation
	expires time.Time
}

// NewPending creates a store. ttl is how long an ID stays valid; max is the
// largest number of questions waiting at once.
func NewPending(ttl time.Duration, max int) *Pending {
	return &Pending{
		items: make(map[string]pendingItem),
		ttl:   ttl,
		max:   max,
		now:   time.Now,
	}
}

// Put stores a conversation and returns its new ID.
func (p *Pending) Put(conv Conversation) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock() // unlocks when Put returns, even on an early return

	p.removeExpired()
	if len(p.items) >= p.max {
		return "", ErrTooManyPending
	}

	// rand.Text returns 26 random characters (Go 1.24+): impossible to guess,
	// so nobody can read someone else's question.
	id := rand.Text()
	p.items[id] = pendingItem{conv: conv, expires: p.now().Add(p.ttl)}
	return id, nil
}

// Take returns the conversation for id and removes it. The second result is
// false if the ID is unknown, already used, or expired.
func (p *Pending) Take(id string) (Conversation, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.items[id]
	if !ok {
		return Conversation{}, false
	}
	delete(p.items, id)
	if p.now().After(item.expires) {
		return Conversation{}, false
	}
	return item.conv, true
}

// Len returns how many questions are waiting (used by tests).
func (p *Pending) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items)
}

// removeExpired deletes old entries. The caller must hold p.mu.
// Deleting from a map while ranging over it is allowed in Go.
func (p *Pending) removeExpired() {
	now := p.now()
	for id, item := range p.items {
		if now.After(item.expires) {
			delete(p.items, id)
		}
	}
}
