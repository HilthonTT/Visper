package middlewares

import "time"

// StrictRateLimiterConfig for sensitive endpoints
func StrictRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerWindow: 10,               // 10 requests
		Window:            time.Minute,      // per minute
		BlockDuration:     time.Minute * 15, // block for 15 minutes
	}
}

// ModerateRateLimiterConfig for normal API endpoints
func ModerateRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerWindow: 60,              // 60 requests
		Window:            time.Minute,     // per minute
		BlockDuration:     time.Minute * 5, // block for 5 minutes
	}
}

// LenientRateLimiterConfig for read-heavy endpoints
func LenientRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerWindow: 200,             // 200 requests
		Window:            time.Minute,     // per minute
		BlockDuration:     time.Minute * 2, // block for 2 minutes
	}
}

// MessageSendingRateLimiterConfig for message sending
func MessageSendingRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		RequestsPerWindow: 30,               // 30 messages
		Window:            time.Minute,      // per minute
		BlockDuration:     time.Minute * 10, // block for 10 minutes
	}
}
