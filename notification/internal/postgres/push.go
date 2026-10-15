package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/pkg/errs"
)

type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{
		pool: pool,
	}
}

// saveSubscriptionQuery takes the browser over from whoever had it and
// keeps the user's newest browsers: $7 besides this one.
const saveSubscriptionQuery = `
	WITH saved AS (
		INSERT INTO push_subscriptions (
			id,
			user_id,
			endpoint,
			p256dh,
			auth,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (endpoint) DO UPDATE
		SET id = EXCLUDED.id,
			user_id = EXCLUDED.user_id,
			p256dh = EXCLUDED.p256dh,
			auth = EXCLUDED.auth,
			created_at = EXCLUDED.created_at
	)
	DELETE FROM push_subscriptions
	WHERE user_id = $2
		AND endpoint <> $3
		AND id NOT IN (
			SELECT id
			FROM push_subscriptions
			WHERE user_id = $2
				AND endpoint <> $3
			ORDER BY id DESC
			LIMIT $7
		)
`

func (r *SubscriptionRepository) Save(ctx context.Context, s *push.Subscription) error {
	keys := s.Keys()

	_, err := r.pool.Exec(ctx, saveSubscriptionQuery,
		s.ID(), s.UserID(), s.Endpoint().String(), keys.P256DH, keys.Auth, s.CreatedAt(), push.MaxSubscriptions-1)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const deleteSubscriptionQuery = `
	DELETE FROM push_subscriptions
	WHERE user_id = $1
		AND endpoint = $2
`

func (r *SubscriptionRepository) Delete(ctx context.Context, userID uuid.UUID, endpoint push.Endpoint) error {
	if _, err := r.pool.Exec(ctx, deleteSubscriptionQuery, userID, endpoint.String()); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const forgetSubscriptionQuery = `
	DELETE FROM push_subscriptions
	WHERE endpoint = $1
`

func (r *SubscriptionRepository) Forget(ctx context.Context, endpoint push.Endpoint) error {
	if _, err := r.pool.Exec(ctx, forgetSubscriptionQuery, endpoint.String()); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const listSubscriptionsQuery = `
	SELECT
		id,
		user_id,
		endpoint,
		p256dh,
		auth,
		created_at
	FROM push_subscriptions
	WHERE user_id = $1
	ORDER BY id DESC
`

func (r *SubscriptionRepository) List(ctx context.Context, userID uuid.UUID) ([]*push.Subscription, error) {
	rows, err := r.pool.Query(ctx, listSubscriptionsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var subs []*push.Subscription

	for rows.Next() {
		var (
			id, owner    uuid.UUID
			endpoint     string
			p256dh, auth []byte
			createdAt    time.Time
		)

		if err = rows.Scan(&id, &owner, &endpoint, &p256dh, &auth, &createdAt); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		// Data comes from our own table and was validated on write.
		subs = append(subs, push.RestoreSubscription(id, owner, push.Endpoint(endpoint), push.Keys{P256DH: p256dh, Auth: auth}, createdAt.UTC()))
	}

	return subs, errs.Wrap("rows.Err", rows.Err())
}

const deleteUserSubscriptionsQuery = `
	DELETE FROM push_subscriptions
	WHERE user_id = $1
`

func (r *SubscriptionRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, deleteUserSubscriptionsQuery, userID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
