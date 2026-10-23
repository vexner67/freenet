package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// PurchaseRepository keeps the paid posts people opened (post_purchases)
// and the messages paid for and not yet known sent (message_payments).
type PurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewPurchaseRepository(pool *pgxpool.Pool) *PurchaseRepository {
	return &PurchaseRepository{
		pool: pool,
	}
}

func (r *PurchaseRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const boughtPostsQuery = `
	SELECT post_id
	FROM post_purchases
	WHERE buyer_id = $1
		AND post_id = ANY($2)
`

func (r *PurchaseRepository) Bought(ctx context.Context, buyerID uuid.UUID, postIDs []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, boughtPostsQuery, buyerID, postIDs)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	bought, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])

	return bought, errs.Wrap("pgx.CollectRows", err)
}

const addPurchaseQuery = `
	INSERT INTO post_purchases (
		post_id,
		buyer_id,
		author_id,
		price
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (post_id, buyer_id) DO NOTHING
`

func (r *PurchaseRepository) AddPurchase(ctx context.Context, buyerID, postID, authorID uuid.UUID, price int64) (bool, error) {
	tag, err := r.conn(ctx).Exec(ctx, addPurchaseQuery, postID, buyerID, authorID, price)
	if err != nil {
		return false, errs.Wrap("r.conn.Exec", err)
	}

	return tag.RowsAffected() == 1, nil
}

const saveMessagePaymentQuery = `
	INSERT INTO message_payments (
		id,
		sender_id,
		recipient_id,
		price,
		tax
	)
	VALUES ($1, $2, $3, $4, $5)
`

func (r *PurchaseRepository) SavePayment(ctx context.Context, p user.MessagePayment) error {
	_, err := r.conn(ctx).Exec(ctx, saveMessagePaymentQuery, p.ID, p.SenderID, p.RecipientID, p.Price, p.Tax)

	return errs.Wrap("r.conn.Exec", err)
}

const takeMessagePaymentQuery = `
	DELETE FROM message_payments
	WHERE id = $1
		AND sender_id = $2
	RETURNING
		id,
		sender_id,
		recipient_id,
		price,
		tax
`

func (r *PurchaseRepository) TakePayment(ctx context.Context, id, senderID uuid.UUID) (user.MessagePayment, error) {
	var p user.MessagePayment

	err := r.conn(ctx).QueryRow(ctx, takeMessagePaymentQuery, id, senderID).Scan(&p.ID, &p.SenderID, &p.RecipientID, &p.Price, &p.Tax)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.MessagePayment{}, user.ErrPaymentNotFound
	}
	if err != nil {
		return user.MessagePayment{}, errs.Wrap("row.Scan", err)
	}

	return p, nil
}
