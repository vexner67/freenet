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
		anonymous,
		number,
		model,
		backdrop,
		symbol,
		sale_price
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`

func (r *GiftRepository) Save(ctx context.Context, g *user.Gift) error {
	number, model, backdrop, symbol, price := collectibleArgs(g)

	_, err := r.conn(ctx).Exec(ctx, saveGiftQuery,
		g.ID(), nullID(g.RecipientID()), nullID(g.SenderID()), g.Kind().String(), g.Caption().String(), g.Anonymous(),
		number, model, backdrop, symbol, price)

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
		anonymous,
		number,
		model,
		backdrop,
		symbol,
		sale_price`

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
		id                      uuid.UUID
		recipientID, senderID   *uuid.UUID
		kind, caption           string
		anonymous               bool
		number                  *int
		model, backdrop, symbol *string
		salePrice               *int64
	)

	if err := row.Scan(&id, &recipientID, &senderID, &kind, &caption, &anonymous,
		&number, &model, &backdrop, &symbol, &salePrice); err != nil {
		return nil, err
	}

	// No recipient: the site holds it. The sender's page is erased: the
	// gift is nobody's.
	g := user.RestoreGift(id, idOrNil(recipientID), idOrNil(senderID), user.GiftKind(kind), user.GiftCaption(caption), anonymous)

	if number == nil {
		return g, nil
	}

	traits := user.GiftTraits{Model: *model, Backdrop: *backdrop, Symbol: *symbol}

	var price int64
	if salePrice != nil {
		price = *salePrice
	}

	return g.RestoreCollectible(*number, traits, price), nil
}

// collectibleArgs are the collectible columns of g, NULL for a plain gift
// and for one not on sale.
func collectibleArgs(g *user.Gift) (number *int, model, backdrop, symbol *string, price *int64) {
	if !g.IsCollectible() {
		return nil, nil, nil, nil, nil
	}

	n, t := g.Number(), g.Traits()
	number, model, backdrop, symbol = &n, &t.Model, &t.Backdrop, &t.Symbol

	if g.IsOnSale() {
		p := g.SalePrice()
		price = &p
	}

	return number, model, backdrop, symbol, price
}

func nullID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

func idOrNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}

	return *id
}

const listGiftPricesQuery = `
	SELECT
		gift,
		price
	FROM gift_prices
`

func (r *GiftRepository) Prices(ctx context.Context) (user.GiftPrices, error) {
	rows, err := r.conn(ctx).Query(ctx, listGiftPricesQuery)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	prices := make(user.GiftPrices)

	for rows.Next() {
		var (
			kind  string
			price int64
		)

		if err := rows.Scan(&kind, &price); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		prices[user.GiftKind(kind)] = user.GiftPrice(price)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return prices, nil
}

const setGiftPriceQuery = `
	INSERT INTO gift_prices (
		gift,
		price
	)
	VALUES ($1, $2)
	ON CONFLICT (gift) DO UPDATE
	SET price = EXCLUDED.price
`

func (r *GiftRepository) SetPrice(ctx context.Context, kind user.GiftKind, price user.GiftPrice) error {
	if _, err := r.conn(ctx).Exec(ctx, setGiftPriceQuery, kind.String(), price.Int64()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
