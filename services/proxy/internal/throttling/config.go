// Package throttling implements sliding-window rate limiting that is
// enforced locally on every request and, when Redis is available, shared
// across proxy instances.
//
// Each key's local usage is kept in a ring of Spans counters covering one
// Interval. At every span boundary the counts of the span that just ended are
// added to a per-interval Redis counter; any key whose global count exceeds
// MaxRequests is blocked for Cooldown. Between syncs each instance limits a
// key to its estimated share (MaxRequests / number of instances).
package throttling

import "time"

// RedisPrefix namespaces every Redis key written by the throttler.
const RedisPrefix = "visper-proxy"

// Config configures a Throttler.
type Config struct {
	// Name namespaces the Redis keys so throttlers with different limits
	// never share counters.
	Name string
	// MaxRequests is the number of requests (weights) allowed per key per Interval.
	MaxRequests int
	Interval    time.Duration
	// Spans is the number of sub-windows per Interval; it sets both the
	// sliding-window granularity and how often counts are synced to Redis.
	Spans int
	// Cooldown is the minimum time a key stays blocked once over its limit.
	Cooldown time.Duration
}

// DefaultConfig returns the defaults applied to zero Config fields.
func DefaultConfig() Config {
	return Config{
		MaxRequests: 100,
		Interval:    time.Minute,
		Spans:       6,
		Cooldown:    30 * time.Second,
	}
}

// normalize fills defaults and returns the config together with the span
// duration. Interval is rounded down to an exact multiple of the span: spans
// are numbered from the Unix epoch, so any remainder would accumulate every
// interval and drift span positions away from interval boundaries.
func (c Config) normalize() (Config, time.Duration) {
	def := DefaultConfig()
	if c.MaxRequests <= 0 {
		c.MaxRequests = def.MaxRequests
	}
	if c.Interval <= 0 {
		c.Interval = def.Interval
	}
	if c.Spans <= 0 {
		c.Spans = def.Spans
	}
	if c.Cooldown < 0 {
		c.Cooldown = 0
	}
	span := c.Interval / time.Duration(c.Spans)
	if span <= 0 {
		span = c.Interval
		c.Spans = 1
	}
	c.Interval = span * time.Duration(c.Spans)
	return c, span
}
