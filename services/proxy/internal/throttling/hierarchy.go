package throttling

import (
	"context"
	"net/http"
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

	errc := make(chan error, len(h.levels))
	for _, l := range h.levels {
		go func() { errc <- l.Throttler.Start(ctx) }()
	}
	err := <-errc // first result (including ctx cancellation) wins
	cancel()
	for range len(h.levels) - 1 {
		<-errc
	}
	return err
}

// Allow checks the levels in order and stops at the first that denies r.
func (h *Hierarchy) Allow(r *http.Request) Decision {
	for _, l := range h.levels {
		if ok, retry := l.Throttler.Allow(l.Key(r), l.Weight); !ok {
			return Decision{Level: l.Name, RetryAfter: retry}
		}
	}
	return Decision{Allowed: true}
}
