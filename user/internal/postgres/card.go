package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// CardRepository keeps the virtual cards of pages.
type CardRepository struct {
	pool *pgxpool.Pool
}

func NewCardRepository(pool *pgxpool.Pool) *CardRepository {
	return &CardRepository{pool: pool}
}

func (r *CardRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

func scanCard(row pgx.Row) (user.BankCard, error) {
	var (
		number, cover string
		issuedAt      time.Time
	)

	if err := row.Scan(&number, &cover, &issuedAt); err != nil {
		return user.BankCard{}, err //nolint:wrapcheck // the caller says what it means
	}

	return user.RestoreBankCard(user.CardNumber(number), user.CardCover(cover), issuedAt.UTC()), nil
}

const cardQuery = `
	SELECT
		number,
		cover,
		issued_at
	FROM bank_cards
	WHERE user_id = $1
`

func (r *CardRepository) Card(ctx context.Context, userID uuid.UUID) (user.BankCard, error) {
	c, err := scanCard(r.conn(ctx).QueryRow(ctx, cardQuery, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.BankCard{}, user.ErrNoCard
	}
	if err != nil {
		return user.BankCard{}, errs.Wrap("scanCard", err)
	}

	return c, nil
}

// addCardQuery leaves the card a page has: only the conflict on user_id is
// expected, one on the number stays an error.
const addCardQuery = `
	INSERT INTO bank_cards (
		user_id,
		number,
		cover,
		issued_at
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (user_id) DO NOTHING
`

func (r *CardRepository) AddCard(ctx context.Context, userID uuid.UUID, c user.BankCard) (user.BankCard, error) {
	_, err := r.conn(ctx).Exec(ctx, addCardQuery, userID, c.Number().String(), c.Cover().String(), c.IssuedAt())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case foreignKeyViolationCode:
			return user.BankCard{}, user.ErrNotFound
		case uniqueViolationCode:
			return user.BankCard{}, user.ErrCardNumberTaken
		}
	}
	if err != nil {
		return user.BankCard{}, errs.Wrap("r.conn.Exec", err)
	}

	return r.Card(ctx, userID)
}

const setCardCoverQuery = `
	UPDATE bank_cards
	SET cover = $2
	WHERE user_id = $1
	RETURNING
		number,
		cover,
		issued_at
`

func (r *CardRepository) SetCardCover(ctx context.Context, userID uuid.UUID, cover user.CardCover) (user.BankCard, error) {
	c, err := scanCard(r.conn(ctx).QueryRow(ctx, setCardCoverQuery, userID, cover.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.BankCard{}, user.ErrNoCard
	}
	if err != nil {
		return user.BankCard{}, errs.Wrap("scanCard", err)
	}

	return c, nil
}
