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
		BlockDuration:     time.Minute * 5, // block or 5 minutes if exceeded
	}
}

func RateLimiterMiddleware(redisClient *redis.Client, logger *logger.Logger, config RateLimiterConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := GetUserFromContext(c)
		if !exists {
			// user_middleware handles this
			c.Next()
			return
		}

		ctx := c.Request.Context()

		if isBlocked, err := checkIfBlocked(ctx, redisClient, user.ID); err != nil {
			logger.Error("failed to check if user is blocked", zap.Error(err), zap.String("userID", user.ID))
			c.Next()
			return
		} else if isBlocked {
			remaining, _ := getBlockTimeRemaining(ctx, redisClient, user.ID)
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.RequestsPerWindow))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(remaining).Unix()))
			c.Header("Retry-After", fmt.Sprintf("%d", int(remaining.Seconds())))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded",
				"message":     "Too many requests. You have been temporarily blocked.",
				"retry_after": int(remaining.Seconds()),
			})
			c.Abort()
			return
		}

		allowed, remaining, resetTime, err := checkRateLimit(ctx, redisClient, user.ID, config)
		if err != nil {
			logger.Error("failed to check rate limit", zap.Error(err), zap.String("userID", user.ID))
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.RequestsPerWindow))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
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

func acquireLock(ctx context.Context, client *redis.Client, lockKey string, ttl time.Duration) (bool, error) {
	return client.SetNX(ctx, lockKey, "1", ttl).Result()
}

func releaseLock(ctx context.Context, client *redis.Client, lockKey string) error {
	return client.Del(ctx, lockKey).Err()
}

func checkRateLimit(ctx context.Context, client *redis.Client, userID string, config RateLimiterConfig) (allowed bool, remaining int, resetTime time.Time, err error) {
	lockKey := fmt.Sprintf("ratelimit:lock:%s", userID)
	key := fmt.Sprintf("ratelimit:%s", userID)

	maxRetries := 10
	retryDelay := 10 * time.Millisecond

	for i := range maxRetries {
		locked, err := acquireLock(ctx, client, lockKey, 2*time.Second)
		if err != nil {
			return false, 0, time.Time{}, fmt.Errorf("failed to acquire lock: %w", err)
		}

		if locked {
			defer releaseLock(ctx, client, lockKey)
			break
		}

		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		} else {
			return false, 0, time.Time{}, fmt.Errorf("could not acquire lock after %d retries", maxRetries)
		}
	}

	now := time.Now()
	windowStart := now.Add(-config.Window)

	pipe := client.Pipeline()

	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	countCmd := pipe.ZCard(ctx, key)

	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	pipe.Expire(ctx, key, config.Window+time.Minute)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, 0, time.Time{}, err
	}

	currentCount := countCmd.Val()
	remaining = max(int(config.RequestsPerWindow)-int(currentCount)-1, 0)

	resetTime = now.Add(config.Window)

	allowed = currentCount < int64(config.RequestsPerWindow)

	return allowed, remaining, resetTime, nil
}

func checkIfBlocked(ctx context.Context, client *redis.Client, userID string) (bool, error) {
	key := fmt.Sprintf("ratelimit:block:%s", userID)
	exists, err := client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

func getBlockTimeRemaining(ctx context.Context, client *redis.Client, userID string) (time.Duration, error) {
	key := fmt.Sprintf("ratelimit:block:%s", userID)
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if ttl < 0 {
		return 0, err
	}
	return ttl, nil
}

func blockUser(ctx context.Context, client *redis.Client, userID string, duration time.Duration) error {
	key := fmt.Sprintf("ratelimit:block:%s", userID)
	return client.Set(ctx, key, "1", duration).Err()
}
