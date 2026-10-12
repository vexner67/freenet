package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

type GiftRepository struct {
	pool *pgxpool.Pool
}

func NewGiftRepository(pool *pgxpool.Pool) *GiftRepository {
	return &GiftRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *GiftRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveGiftQuery = `
	INSERT INTO gifts (
		id,
		recipient_id,
		sender_id,
		gift,
		caption,
		anonymous
	)
	VALUES ($1, $2, $3, $4, $5, $6)
`

func (r *GiftRepository) Save(ctx context.Context, g *user.Gift) error {
	_, err := r.conn(ctx).Exec(ctx, saveGiftQuery,
		g.ID(), g.RecipientID(), g.SenderID(), g.Kind().String(), g.Caption().String(), g.Anonymous())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		// One of the two was erased between the check and the insert.
		return user.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const giftColumns = `
		id,
		recipient_id,
		sender_id,
		gift,
		caption,
		anonymous`

const getGiftQuery = `
	SELECT` + giftColumns + `
	FROM gifts
	WHERE id = $1
`

func (r *GiftRepository) Get(ctx context.Context, id uuid.UUID) (*user.Gift, error) {
	g, err := scanGift(r.conn(ctx).QueryRow(ctx, getGiftQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrGiftNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanGift", err)
	}

	return g, nil
}

const deleteGiftQuery = `
	DELETE FROM gifts
	WHERE id = $1
`

func (r *GiftRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteGiftQuery, id); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const listGiftsQuery = `
	SELECT` + giftColumns + `
	FROM gifts
	WHERE recipient_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *GiftRepository) List(ctx context.Context, recipientID, before uuid.UUID, limit int) ([]*user.Gift, error) {
	if before == uuid.Nil {
		// Above every UUID: start from the newest gift.
		before = uuid.Max
	}

	rows, err := r.conn(ctx).Query(ctx, listGiftsQuery, recipientID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var gifts []*user.Gift

	for rows.Next() {
		g, err := scanGift(rows)
		if err != nil {
			return nil, errs.Wrap("scanGift", err)
		}

		gifts = append(gifts, g)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return gifts, nil
}

const countGiftsQuery = `
	SELECT count(*)
	FROM gifts
	WHERE recipient_id = $1
`

func (r *GiftRepository) Count(ctx context.Context, recipientID uuid.UUID) (int, error) {
	var count int

	if err := r.conn(ctx).QueryRow(ctx, countGiftsQuery, recipientID).Scan(&count); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return count, nil
}

func scanGift(row pgx.Row) (*user.Gift, error) {
	var (
		id, recipientID uuid.UUID
		senderID        *uuid.UUID
		kind, caption   string
		anonymous       bool
	)

	if err := row.Scan(&id, &recipientID, &senderID, &kind, &caption, &anonymous); err != nil {
		return nil, err
	}

	// The sender's page is erased: the gift is nobody's.
	sender := uuid.Nil
	if senderID != nil {
		sender = *senderID
	}

	return user.RestoreGift(id, recipientID, sender, user.GiftKind(kind), user.GiftCaption(caption), anonymous), nil
}
