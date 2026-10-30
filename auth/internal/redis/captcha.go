package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

func captchaKey(id string) string {
	return "captcha:" + id
}

// CaptchaRepository keeps the pictures waiting to be read.
type CaptchaRepository struct {
	client *redis.Client
}

func NewCaptchaRepository(client *redis.Client) *CaptchaRepository {
	return &CaptchaRepository{client: client}
}

func (r *CaptchaRepository) Save(ctx context.Context, id, answerHash string, ttl time.Duration) error {
	return errs.Wrap("r.client.Set", r.client.Set(ctx, captchaKey(id), answerHash, ttl).Err())
}

// Take reads and deletes in one step: two answers cannot share a picture.
func (r *CaptchaRepository) Take(ctx context.Context, id string) (string, error) {
	hash, err := r.client.GetDel(ctx, captchaKey(id)).Result()
	if errors.Is(err, redis.Nil) {
		return "", service.ErrCaptcha
	}
	if err != nil {
		return "", errs.Wrap("r.client.GetDel", err)
	}

	return hash, nil
}
