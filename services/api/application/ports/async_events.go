package ports

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// AsyncEvents publishes events in the background so requests don't wait on
// the broker, and lets shutdown wait for the ones still in flight. A nil
// *AsyncEvents, or one without a publisher, drops every event.
type AsyncEvents struct {
	publisher EventPublisher
	logger    Logger
	wg        sync.WaitGroup
}

// NewAsyncEvents returns an AsyncEvents publishing to publisher (which may be
// nil) and logging failures to logger.
func NewAsyncEvents(publisher EventPublisher, logger Logger) *AsyncEvents {
	return &AsyncEvents{publisher: publisher, logger: logger}
}

// Publish runs publish in the background; a failure is logged under event.
func (a *AsyncEvents) Publish(event string, publish func(EventPublisher) error) {
	if a == nil || a.publisher == nil {
		return
	}
	a.wg.Go(func() {
		if err := publish(a.publisher); err != nil {
			a.logger.Error("failed to publish event", zap.String("event", event), zap.Error(err))
		}
	})
}

// Wait waits for the events in flight to be published, or for ctx to end.
func (a *AsyncEvents) Wait(ctx context.Context) error {
	if a == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
