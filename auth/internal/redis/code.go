package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type CodeRepository struct {
	client *redis.Client
}

func NewCodeRepository(client *redis.Client) *CodeRepository {
	return &CodeRepository{
		client: client,
	}
}

func (r *CodeRepository) Save(
	ctx context.Context,
	email string,
	codeHash string,
	ttl time.Duration,
) error {
	return r.client.Set(
		ctx,
		"verification_code:"+email,
		codeHash,
		ttl,
	).Err()
}

func (r *CodeRepository) Get(ctx context.Context, email string) (string, error) {
	return r.client.Get(
		ctx,
		"verification_code:"+email,
	).Result()
}

func (r *CodeRepository) Delete(ctx context.Context, email string) error {
	return r.client.Del(
		ctx,
		"verification_code:"+email,
	).Err()
}
