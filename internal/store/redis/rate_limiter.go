package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rateLimitPrefix = "chat:ratelimit:msg:"
	rateLimitWindow = 1 * time.Minute
	rateLimitMax    = 60 // max messages per window
)

// RateLimiter implements service.RateLimiter using Redis.
type RateLimiter struct {
	client *redis.Client
}

// NewRateLimiter creates a new Redis rate limiter.
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

func (r *RateLimiter) AllowMessage(ctx context.Context, userID string) (bool, error) {
	key := rateLimitPrefix + userID

	pipe := r.client.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rateLimitWindow)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("rate limit check: %w", err)
	}

	count := incrCmd.Val()
	return count <= rateLimitMax, nil
}
