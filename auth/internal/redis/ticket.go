package redis

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// A ticket and its attempts live in two keys with the same lifetime, as a
// code and its attempts do.
func ticketKey(hash string) string {
	return "two_factor_ticket:" + hash
}

func ticketAttemptsKey(hash string) string {
	return "two_factor_ticket_attempts:" + hash
}

// TicketRepository keeps the sign-ins waiting for their second step.
type TicketRepository struct {
	client *redis.Client
}

func NewTicketRepository(client *redis.Client) *TicketRepository {
	return &TicketRepository{client: client}
}

func (r *TicketRepository) Save(ctx context.Context, hash string, userID uuid.UUID, ttl time.Duration) error {
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, ticketKey(hash), userID.String(), ttl)
		pipe.Del(ctx, ticketAttemptsKey(hash))
		return nil
	})

	return errs.Wrap("r.client.TxPipelined", err)
}

func (r *TicketRepository) Get(ctx context.Context, hash string) (uuid.UUID, error) {
	value, err := r.client.Get(ctx, ticketKey(hash)).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, service.ErrTicketNotFound
	}
	if err != nil {
		return uuid.Nil, errs.Wrap("r.client.Get", err)
	}

	id, err := uuid.Parse(value)

	return id, errs.Wrap("uuid.Parse", err)
}

func (r *TicketRepository) IncrementAttempts(ctx context.Context, hash string) (int, error) {
	// The script of the codes: it counts only while the ticket lives.
	attempts, err := incrementAttemptsScript.Run(ctx, r.client, []string{ticketKey(hash), ticketAttemptsKey(hash)}).Int()
	if err != nil {
		return 0, errs.Wrap("incrementAttemptsScript.Run", err)
	}

	if attempts < 0 {
		return 0, service.ErrTicketNotFound
	}

	return attempts, nil
}

func (r *TicketRepository) Delete(ctx context.Context, hash string) error {
	var deleted *redis.IntCmd

	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		deleted = pipe.Del(ctx, ticketKey(hash))
		pipe.Del(ctx, ticketAttemptsKey(hash))
		return nil
	})
	if err != nil {
		return errs.Wrap("r.client.TxPipelined", err)
	}

	if deleted.Val() == 0 {
		return service.ErrTicketNotFound
	}

	return nil
}
