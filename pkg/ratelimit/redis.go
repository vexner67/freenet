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
	// owned: Connect opened the client, so Close closes it.
	owned bool
}

// Connect dials Redis at addr for a limiter of its own; Close it when done.
func Connect(ctx context.Context, addr string) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errs.Wrap("client.Ping", err)
	}

	return &Redis{client: client, owned: true}, nil
}

// Close closes the connection Connect opened; a client passed to NewRedis
// belongs to the caller and stays open.
func (l *Redis) Close() error {
	if !l.owned {
		return nil
	}

	return l.client.Close()
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

// Store is a limiter to close when done.
type Store interface {
	Limiter
	Close() error
}

// Open counts in Redis at addr, shared by all replicas, or in memory when
// addr is empty.
func Open(ctx context.Context, addr string) (Store, error) {
	if addr == "" {
		return NewMemory(), nil
	}

	return Connect(ctx, addr)
}
