package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// A code and its attempts live in two keys with the same lifetime.
func codeKey(email account.Email) string {
	return "verification_code:" + email.String()
}

func attemptsKey(email account.Email) string {
	return "verification_attempts:" + email.String()
}

// incrementAttemptsScript counts an attempt only while the code exists and
// lets the counter expire together with it. Returns -1 without a code.
var incrementAttemptsScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
	return -1
end
local attempts = redis.call('INCR', KEYS[2])
if attempts == 1 then
	redis.call('PEXPIRE', KEYS[2], redis.call('PTTL', KEYS[1]))
end
return attempts
`)

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
	email account.Email,
	codeHash string,
	ttl time.Duration,
) error {
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, codeKey(email), codeHash, ttl)
		pipe.Del(ctx, attemptsKey(email))
		return nil
	})
	if err != nil {
		return errs.Wrap("r.client.TxPipelined", err)
	}

	return nil
}

func (r *CodeRepository) Get(ctx context.Context, email account.Email) (string, error) {
	hash, err := r.client.Get(ctx, codeKey(email)).Result()
	if errors.Is(err, redis.Nil) {
		return "", service.ErrCodeNotFound
	}
	if err != nil {
		return "", errs.Wrap("r.client.Get", err)
	}

	return hash, nil
}

func (r *CodeRepository) IncrementAttempts(ctx context.Context, email account.Email) (int, error) {
	attempts, err := incrementAttemptsScript.Run(
		ctx,
		r.client,
		[]string{codeKey(email), attemptsKey(email)},
	).Int()
	if err != nil {
		return 0, errs.Wrap("incrementAttemptsScript.Run", err)
	}

	if attempts < 0 {
		return 0, service.ErrCodeNotFound
	}

	return attempts, nil
}

func (r *CodeRepository) Delete(ctx context.Context, email account.Email) error {
	var deleted *redis.IntCmd

	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		deleted = pipe.Del(ctx, codeKey(email))
		pipe.Del(ctx, attemptsKey(email))
		return nil
	})
	if err != nil {
		return errs.Wrap("r.client.TxPipelined", err)
	}

	if deleted.Val() == 0 {
		return service.ErrCodeNotFound
	}

	return nil
}
