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

// WalletRepository changes the votes and the rating kept in the rows of
// users.
type WalletRepository struct {
	pool *pgxpool.Pool
}

func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *WalletRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// The balance is checked by the statement that changes it, so two spends
// at once cannot both take the last votes. The first column tells a user
// who is gone from one who has too few.
const spendVotesQuery = `
	WITH spent AS (
		UPDATE users
		SET votes = votes - $2
		WHERE id = $1
			AND votes >= $2
		RETURNING votes
	)
	SELECT
		EXISTS (SELECT 1 FROM users WHERE id = $1),
		(SELECT votes FROM spent)
`

func (r *WalletRepository) Spend(ctx context.Context, userID uuid.UUID, votes int64) (int64, error) {
	var (
		exists bool
		left   *int64
	)

	if err := r.conn(ctx).QueryRow(ctx, spendVotesQuery, userID, votes).Scan(&exists, &left); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	switch {
	case !exists:
		return 0, user.ErrNotFound
	case left == nil:
		return 0, user.ErrNotEnoughVotes
	default:
		return *left, nil
	}
}

// Both rows are locked in the order of their IDs before either changes:
// two people sending votes to each other at once would otherwise wait for
// each other.
const lockWalletsQuery = `
	SELECT id
	FROM users
	WHERE id IN ($1, $2)
	ORDER BY id
	FOR UPDATE
`

const receiveVotesQuery = `
	UPDATE users
	SET votes = votes + $2
	WHERE id = $1
`

func (r *WalletRepository) Move(ctx context.Context, fromID, toID uuid.UUID, votes int64) (int64, error) {
	conn := r.conn(ctx)

	rows, err := conn.Query(ctx, lockWalletsQuery, fromID, toID)
	if err != nil {
		return 0, errs.Wrap("conn.Query", err)
	}

	locked, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, errs.Wrap("pgx.CollectRows", err)
	}

	if len(locked) != 2 {
		return 0, user.ErrNotFound
	}

	left, err := r.Spend(ctx, fromID, votes)
	if err != nil {
		return 0, errs.Wrap("r.Spend", err)
	}

	if _, err = conn.Exec(ctx, receiveVotesQuery, toID, votes); err != nil {
		return 0, errs.Wrap("conn.Exec", err)
	}

	return left, nil
}

const creditWalletQuery = `
	UPDATE users
	SET
		votes = votes + $2,
		rating = rating + $3
	WHERE id = $1
	RETURNING
		votes,
		rating
`

func (r *WalletRepository) Credit(ctx context.Context, userID uuid.UUID, votes int64, rating int) (user.Wallet, error) {
	var w user.Wallet

	err := r.conn(ctx).QueryRow(ctx, creditWalletQuery, userID, votes, rating).Scan(&w.Votes, &w.Rating)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.Wallet{}, user.ErrNotFound
	}
	if err != nil {
		return user.Wallet{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return w, nil
}

type VoucherRepository struct {
	pool *pgxpool.Pool
}

func NewVoucherRepository(pool *pgxpool.Pool) *VoucherRepository {
	return &VoucherRepository{
		pool: pool,
	}
}

func (r *VoucherRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveVoucherQuery = `
	INSERT INTO vouchers (
		id,
		token,
		votes,
		rating,
		usages_left
	)
	VALUES ($1, $2, $3, $4, $5)
`

func (r *VoucherRepository) Save(ctx context.Context, v *user.Voucher) error {
	_, err := r.conn(ctx).Exec(ctx, saveVoucherQuery,
		v.ID(), v.Token().String(), v.Votes(), v.Rating(), usagesToDB(v.UsagesLeft()))

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
		return user.ErrVoucherExists
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// The voucher is locked first: a second account after the last usage
// waits, reads the row again and finds none left. An account that has
// used it inserts nothing, and the voucher is then not returned.
const redeemVoucherQuery = `
	WITH voucher AS (
		SELECT
			id,
			votes,
			rating
		FROM vouchers
		WHERE token = $1
			AND (usages_left IS NULL OR usages_left > 0)
		FOR UPDATE
	), used AS (
		INSERT INTO voucher_users (
			voucher_id,
			user_id
		)
		SELECT
			id,
			$2
		FROM voucher
		ON CONFLICT DO NOTHING
		RETURNING voucher_id
	), spent AS (
		UPDATE vouchers v
		SET usages_left = v.usages_left - 1
		FROM used
		WHERE v.id = used.voucher_id
			AND v.usages_left IS NOT NULL
	)
	SELECT
		voucher.id,
		voucher.votes,
		voucher.rating
	FROM voucher
	JOIN used ON used.voucher_id = voucher.id
`

func (r *VoucherRepository) Redeem(ctx context.Context, token user.VoucherToken, userID uuid.UUID) (*user.Voucher, error) {
	s := user.StoredVoucher{Token: token}

	err := r.conn(ctx).QueryRow(ctx, redeemVoucherQuery, token.String(), userID).Scan(&s.ID, &s.Votes, &s.Rating)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrVoucherInvalid
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		// The user was erased before the voucher was used.
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}

	return user.RestoreVoucher(s), nil
}

const listVouchersQuery = `
	SELECT
		v.id,
		v.token,
		v.votes,
		v.rating,
		v.usages_left,
		(SELECT count(*) FROM voucher_users vu WHERE vu.voucher_id = v.id)
	FROM vouchers v
	WHERE v.id < $1
	ORDER BY v.id DESC
	LIMIT $2
`

func (r *VoucherRepository) List(ctx context.Context, before uuid.UUID, limit int) ([]*user.Voucher, error) {
	if before == uuid.Nil {
		// Above every UUID: start from the newest voucher.
		before = uuid.Max
	}

	rows, err := r.conn(ctx).Query(ctx, listVouchersQuery, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var vouchers []*user.Voucher

	for rows.Next() {
		var (
			s          user.StoredVoucher
			token      string
			usagesLeft *int
		)

		if err = rows.Scan(&s.ID, &token, &s.Votes, &s.Rating, &usagesLeft, &s.Activations); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		s.Token, s.UsagesLeft = user.VoucherToken(token), usagesFromDB(usagesLeft)
		vouchers = append(vouchers, user.RestoreVoucher(s))
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return vouchers, nil
}

const deleteVoucherQuery = `
	DELETE FROM vouchers
	WHERE id = $1
`

func (r *VoucherRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteVoucherQuery, id); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// A voucher for any number of accounts keeps no count.
func usagesToDB(usages int) *int {
	if usages == user.UnlimitedUsages {
		return nil
	}

	return &usages
}

func usagesFromDB(usages *int) int {
	if usages == nil {
		return user.UnlimitedUsages
	}

	return *usages
}
