package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/auth/internal/errs"
)

func New(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errs.Wrap("client.Ping", err)
	}

	return client, nil
}
