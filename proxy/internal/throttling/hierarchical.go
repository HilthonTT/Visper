package throttling

import (
	"context"
	"net/http"
)

type ThrottleLevel struct {
	Name         string
	KeyExtractor func(r *http.Request) string
	Throttler    *Throttler
	Weight       int // request cost; 1 for most routes
}

type HierarchicalThrottler struct {
	levels []*ThrottleLevel
}

func NewHierarchicalThrottler(levels ...*ThrottleLevel) *HierarchicalThrottler {
	return &HierarchicalThrottler{levels: levels}
}

// Start launches all throttler background goroutines and blocks until ctx is
// cancelled or a throttler fails. All goroutines have exited when it returns.
func (h *HierarchicalThrottler) Start(ctx context.Context) error {
	if len(h.levels) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errc := make(chan error, len(h.levels))
	for _, level := range h.levels {
		go func(l *ThrottleLevel) {
			errc <- l.Throttler.Start(ctx)
		}(level)
	}
	// Return first error (including ctx cancellation), then stop the rest.
	err := <-errc
	cancel()
	for range len(h.levels) - 1 {
		<-errc //nolint:errcheck // remaining results are from the cancellation above
	}
	return err
}

// CheckRequest checks levels in order and returns false together with the
// denying level as soon as any level denies the request. Order levels from
// most to least specific so a blocked client does not consume shared quota.
func (h *HierarchicalThrottler) CheckRequest(r *http.Request) (bool, *ThrottleLevel) {
	for _, level := range h.levels {
		key := level.KeyExtractor(r)
		weight := level.Weight
		if weight <= 0 {
			weight = 1
		}
		if !level.Throttler.IncrementAndCheck(key, weight) {
			return false, level
		}
	}
	return true, nil
}
