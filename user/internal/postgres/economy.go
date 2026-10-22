package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// EconomyRepository keeps the economy of the site: the one row of economy.
type EconomyRepository struct {
	pool *pgxpool.Pool
}

func NewEconomyRepository(pool *pgxpool.Pool) *EconomyRepository {
	return &EconomyRepository{
		pool: pool,
	}
}

func (r *EconomyRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const getEconomyQuery = `
	SELECT
		bonus,
		transfer_fee,
		sticker_tax,
		prime_sticker_tax
	FROM economy
`

func (r *EconomyRepository) Economy(ctx context.Context) (user.Economy, error) {
	var e user.Economy

	err := r.conn(ctx).QueryRow(ctx, getEconomyQuery).Scan(&e.Bonus, &e.TransferFee, &e.StickerTax, &e.PrimeStickerTax)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DefaultEconomy, nil
	}
	if err != nil {
		return user.Economy{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return e, nil
}

const setEconomyQuery = `
	INSERT INTO economy (
		bonus,
		transfer_fee,
		sticker_tax,
		prime_sticker_tax
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (only_row) DO UPDATE
	SET bonus = EXCLUDED.bonus,
		transfer_fee = EXCLUDED.transfer_fee,
		sticker_tax = EXCLUDED.sticker_tax,
		prime_sticker_tax = EXCLUDED.prime_sticker_tax
`

func (r *EconomyRepository) SetEconomy(ctx context.Context, e user.Economy) error {
	if _, err := r.conn(ctx).Exec(ctx, setEconomyQuery, e.Bonus, e.TransferFee, e.StickerTax, e.PrimeStickerTax); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
