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

// Limiting single IPv6 addresses let a client rotate through its /64 for
// fresh budgets.
func TestRateLimitSubjectGroupsIPv6By64(t *testing.T) {
	tests := map[string]string{
		"203.0.113.7":                 "203.0.113.7",
		"::ffff:203.0.113.7":          "203.0.113.7",
		"2001:db8:1:2:aaaa::1":        "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff:ffff::abc": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":             "2001:db8:1:3::/64",
		"not-an-ip":                   "not-an-ip",
	}
	for ip, want := range tests {
		if got := rateLimitSubject(ip); got != want {
			t.Errorf("rateLimitSubject(%q) = %q, want %q", ip, got, want)
		}
	}
}
