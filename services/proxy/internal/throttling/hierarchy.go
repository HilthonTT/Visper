package throttling

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Level is one tier of a Hierarchy, e.g. per client IP or global.
type Level struct {
	Name      string
	Key       func(r *http.Request) string
	Throttler *Throttler
	Weight    int // request cost; 0 means 1
}

// Decision is the outcome of Hierarchy.Allow.
type Decision struct {
	Allowed bool
	// Level is the name of the denying level; empty when allowed.
	Level string
	// RetryAfter is how long the denying level keeps the key blocked.
	RetryAfter time.Duration
}

// Hierarchy checks a request against several throttling levels in order.
type Hierarchy struct {
	levels []*Level
}

// NewHierarchy creates a Hierarchy. Order levels from most to least specific
// so a blocked client does not consume shared quota.
func NewHierarchy(levels ...*Level) *Hierarchy {
	return &Hierarchy{levels: levels}
}

// Start runs every level's throttler and blocks until ctx is cancelled or one
// of them fails. All goroutines have exited when it returns.
func (h *Hierarchy) Start(ctx context.Context) error {
	if len(h.levels) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Buffered so the levels that stop after the first one never block; their
	// errors are only the consequence of the cancel below and are discarded.
	errc := make(chan error, len(h.levels))
	var wg sync.WaitGroup
	for _, l := range h.levels {
		wg.Go(func() { errc <- l.Throttler.Start(ctx) })
	}
	err := <-errc // first result (including ctx cancellation) wins
	cancel()
	wg.Wait()
	return err
}

// Allow checks the levels in order and stops at the first that denies r. A
// denied request consumes no quota: what earlier levels recorded is refunded.
func (h *Hierarchy) Allow(r *http.Request) Decision {
	keys := make([]string, 0, len(h.levels))
	for i, l := range h.levels {
		key := l.Key(r)
		if ok, retry := l.Throttler.Allow(key, l.Weight); !ok {
			for j := range i {
				h.levels[j].Throttler.Refund(keys[j], h.levels[j].Weight)
			}
			return Decision{Level: l.Name, RetryAfter: retry}
		}
		keys = append(keys, key)
	}
	return Decision{Allowed: true}
}
