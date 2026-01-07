package middlewares

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RateLimiterConfig struct {
	RequestsPerWindow int           // Number of requests allowed
	Window            time.Duration // Time window
	BlockDuration     time.Duration // How long to block after exceeding limit
}

func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
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

-- Add current request
redis.call('ZADD', key, now, now)

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

func RateLimiterMiddleware(redisClient *redis.Client, logger *logger.Logger, config RateLimiterConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := GetUserFromContext(c)
		if !exists {
			// user_middleware handles this
			c.Next()
			return
		}

		ctx := c.Request.Context()

		blockKey := fmt.Sprintf("ratelimit:block:%s", user.ID)
		blockResult, err := redisClient.Eval(ctx, checkBlockScript, []string{blockKey}).Result()
		if err != nil {
			logger.Error("failed to check if user is blocked", zap.Error(err), zap.String("userID", user.ID))
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

		allowed, remaining, resetTime, err := checkRateLimitAtomic(ctx, redisClient, user.ID, config)
		if err != nil {
			logger.Error("failed to check rate limit", zap.Error(err), zap.String("userID", user.ID))
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.RequestsPerWindow))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
			// Block user
			if err := blockUser(ctx, redisClient, user.ID, config.BlockDuration); err != nil {
				logger.Error("failed to block user", zap.Error(err), zap.String("userID", user.ID))
			}

			logger.Warn("rate limit exceeded",
				zap.String("userID", user.ID),
				zap.String("username", user.Username),
				zap.String("path", c.Request.URL.Path),
			)

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

func checkRateLimitAtomic(ctx context.Context, client *redis.Client, userID string, config RateLimiterConfig) (allowed bool, remaining int, resetTime time.Time, err error) {
	key := fmt.Sprintf("ratelimit:%s", userID)
	now := time.Now()

	result, err := client.Eval(ctx, rateLimitScript,
		[]string{key},
		now.UnixNano(),
		config.Window.Nanoseconds(),
		config.RequestsPerWindow,
		int(config.Window.Seconds())+60, // expiry buffer
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

func blockUser(ctx context.Context, client *redis.Client, userID string, duration time.Duration) error {
	key := fmt.Sprintf("ratelimit:block:%s", userID)
	return client.Set(ctx, key, "1", duration).Err()
}
