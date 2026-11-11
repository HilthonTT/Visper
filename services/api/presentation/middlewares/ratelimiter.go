package middlewares

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RateLimiterConfig struct {
	// Name namespaces this limiter's Redis keys. Limiters stacked on the same
	// route (e.g. the API-wide one and the stricter /files one) must not share
	// counters: otherwise ordinary requests eat the strict budget, and a
	// strict block locks the client out of the whole API.
	Name              string
	RequestsPerWindow int           // Number of requests allowed
	Window            time.Duration // Time window
	BlockDuration     time.Duration // How long to block after exceeding limit
}

func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		Name:              "default",
		RequestsPerWindow: 150,             // 150 requests
		Window:            time.Minute,     // per minute
		BlockDuration:     time.Minute * 5, // block for 5 minutes if exceeded
	}
}

const rateLimitScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local expiry = tonumber(ARGV[4])

-- Remove old entries outside the window
local windowStart = now - window
redis.call('ZREMRANGEBYSCORE', key, 0, windowStart)

-- Count current requests in window
local currentCount = redis.call('ZCARD', key)

-- Add current request (ARGV[5] is a unique member, so concurrent requests never collapse)
redis.call('ZADD', key, now, ARGV[5])

-- Set expiry on the key
redis.call('EXPIRE', key, expiry)

-- Calculate remaining
local remaining = math.max(limit - currentCount - 1, 0)

-- Check if allowed
local allowed = currentCount < limit

return {allowed and 1 or 0, remaining, currentCount + 1}
`

const checkBlockScript = `
local blockKey = KEYS[1]

local exists = redis.call('EXISTS', blockKey)
if exists == 0 then
    return {0, 0}
end

local ttl = redis.call('TTL', blockKey)
return {1, ttl}
`

// RateLimiterMiddleware limits requests per client IP. It deliberately does
// not key on the user: user IDs come from the client-controlled X-User-ID
// header, so a fresh ID per request would bypass the limit. The IP comes from
// gin's ClientIP, which only honours X-Forwarded-For / X-Real-IP from the
// proxies listed in server.trustedProxies.
func RateLimiterMiddleware(redisClient *redis.Client, logger *logger.Logger, config RateLimiterConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		clientKey := config.keyPrefix() + "ip:" + c.ClientIP()

		blockKey := fmt.Sprintf("ratelimit:block:%s", clientKey)
		blockResult, err := redisClient.Eval(ctx, checkBlockScript, []string{blockKey}).Result()
		if err != nil {
			logger.Error("failed to check if client is blocked", zap.Error(err), zap.String("client", clientKey))
			c.Next()
			return
		}

		blockInfo := blockResult.([]any)
		isBlocked := blockInfo[0].(int64) == 1

		if isBlocked {
			ttl := time.Duration(blockInfo[1].(int64)) * time.Second

			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.RequestsPerWindow))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(ttl).Unix()))
			c.Header("Retry-After", fmt.Sprintf("%d", int(ttl.Seconds())))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded",
				"message":     "Too many requests. You have been temporarily blocked.",
				"retry_after": int(ttl.Seconds()),
			})
			c.Abort()
			return
		}

		allowed, remaining, resetTime, err := checkRateLimitAtomic(ctx, redisClient, clientKey, config)
		if err != nil {
			logger.Error("failed to check rate limit", zap.Error(err), zap.String("client", clientKey))
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.RequestsPerWindow))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
			if err := blockClient(ctx, redisClient, clientKey, config.BlockDuration); err != nil {
				logger.Error("failed to block client", zap.Error(err), zap.String("client", clientKey))
			}

			fields := []zap.Field{zap.String("client", clientKey), zap.String("path", c.Request.URL.Path)}
			if user, ok := GetUserFromContext(c); ok {
				fields = append(fields, zap.String("userID", user.ID))
			}
			logger.Warn("rate limit exceeded", fields...)

			c.Header("Retry-After", fmt.Sprintf("%d", int(config.BlockDuration.Seconds())))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded",
				"message":     fmt.Sprintf("Rate limit exceeded. Maximum %d requests per %v.", config.RequestsPerWindow, config.Window),
				"retry_after": int(config.BlockDuration.Seconds()),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func checkRateLimitAtomic(ctx context.Context, client *redis.Client, clientKey string, config RateLimiterConfig) (allowed bool, remaining int, resetTime time.Time, err error) {
	key := fmt.Sprintf("ratelimit:%s", clientKey)
	now := time.Now()

	result, err := client.Eval(ctx, rateLimitScript,
		[]string{key},
		// Microseconds stay exact as Lua doubles; nanoseconds (~1.8e18) do not.
		now.UnixMicro(),
		config.Window.Microseconds(),
		config.RequestsPerWindow,
		int(config.Window.Seconds())+60, // expiry buffer
		fmt.Sprintf("%d-%d", now.UnixNano(), rand.Uint64()),
	).Result()

	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("rate limit script failed: %w", err)
	}

	resultArray := result.([]any)
	allowedInt := resultArray[0].(int64)
	remainingInt := resultArray[1].(int64)

	allowed = allowedInt == 1
	remaining = int(remainingInt)
	resetTime = now.Add(config.Window)

	return allowed, remaining, resetTime, nil
}

func blockClient(ctx context.Context, client *redis.Client, clientKey string, duration time.Duration) error {
	key := fmt.Sprintf("ratelimit:block:%s", clientKey)
	return client.Set(ctx, key, "1", duration).Err()
}

// keyPrefix scopes Redis keys to this limiter (see RateLimiterConfig.Name).
func (c RateLimiterConfig) keyPrefix() string {
	if c.Name == "" {
		return "default:"
	}
	return c.Name + ":"
}
