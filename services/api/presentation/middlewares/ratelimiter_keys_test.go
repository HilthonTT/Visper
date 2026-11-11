package middlewares

import "testing"

// Stacked limiters (API-wide + /files) used to share Redis counters and block
// keys, so ordinary requests consumed the strict upload budget and a strict
// block locked the client out of the whole API.
func TestRateLimiterConfigsUseDistinctKeyPrefixes(t *testing.T) {
	configs := map[string]RateLimiterConfig{
		"default":         DefaultRateLimiterConfig(),
		"strict":          StrictRateLimiterConfig(),
		"moderate":        ModerateRateLimiterConfig(),
		"lenient":         LenientRateLimiterConfig(),
		"message-sending": MessageSendingRateLimiterConfig(),
	}
	seen := map[string]string{}
	for name, cfg := range configs {
		p := cfg.keyPrefix()
		if other, dup := seen[p]; dup {
			t.Fatalf("%s and %s share the key prefix %q", name, other, p)
		}
		seen[p] = name
	}
	if got := (RateLimiterConfig{}).keyPrefix(); got != "default:" {
		t.Fatalf("unnamed config prefix = %q", got)
	}
}
