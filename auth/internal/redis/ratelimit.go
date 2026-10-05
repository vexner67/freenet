package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/pkg/errs"
)

// RateLimiter keeps fixed-window counters in Redis, shared by all replicas.
type RateLimiter struct {
	client *redis.Client
}

func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{
		client: client,
	}
}

func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	key = "rate_limit:" + key

	var count *redis.IntCmd

	_, err := l.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		count = pipe.Incr(ctx, key)
		// NX: the window starts with the first event and is not extended.
		pipe.ExpireNX(ctx, key, window)
		return nil
	})
	if err != nil {
		return false, errs.Wrap("l.client.TxPipelined", err)
	}

	return count.Val() <= int64(limit), nil
}
