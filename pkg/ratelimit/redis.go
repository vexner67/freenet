package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/pkg/errs"
)

// Redis keeps fixed-window counters in Redis, shared by all replicas.
type Redis struct {
	client *redis.Client
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{
		client: client,
	}
}

// Allow counts an event of key and tells whether it is within limit
// events per window.
func (l *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
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
