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

// lockTreasuryQuery locks the treasury before any row of users: every
// change of votes takes the locks in that order, so two never wait for each
// other in a circle. Every payment of the site goes through it in turn.
const lockTreasuryQuery = `
	SELECT votes
	FROM treasury
	FOR UPDATE
`

// The balance is checked by the statement that changes it, so two spends
// at once cannot both take the last votes; the same statement writes the
// line of the statement of the page and, with $7, puts the votes into the
// treasury. The first column tells a user who is gone from one who has too
// few.
const spendVotesQuery = `
	WITH spent AS (
		UPDATE users
		SET votes = votes - $2
		WHERE id = $1
			AND votes >= $2
		RETURNING votes
	), logged AS (
		INSERT INTO vote_operations (
			id,
			user_id,
			kind,
			amount,
			balance,
			peer_id,
			note
		)
		SELECT
			$3,
			$1,
			$4,
			-$2::bigint,
			votes,
			$5,
			$6
		FROM spent
	), kept AS (
		UPDATE treasury
		SET votes = votes + $2
		WHERE $7
			AND EXISTS (SELECT 1 FROM spent)
	)
	SELECT
		EXISTS (SELECT 1 FROM users WHERE id = $1),
		(SELECT votes FROM spent)
`

// Spend puts the votes spent into the treasury.
func (r *WalletRepository) Spend(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error) {
	if err := r.lockTreasury(ctx); err != nil {
		return 0, errs.Wrap("r.lockTreasury", err)
	}

	left, err := r.spend(ctx, userID, votes, e, true)

	return left, errs.Wrap("r.spend", err)
}

func (r *WalletRepository) lockTreasury(ctx context.Context) error {
	var votes int64
	if err := r.conn(ctx).QueryRow(ctx, lockTreasuryQuery).Scan(&votes); err != nil {
		return errs.Wrap("r.conn.QueryRow", err)
	}

	return nil
}

// spend takes votes from userID, into the treasury if treasured.
func (r *WalletRepository) spend(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry, treasured bool) (int64, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return 0, errs.Wrap("uuid.NewV7", err)
	}

	var (
		exists bool
		left   *int64
	)

	err = r.conn(ctx).QueryRow(ctx, spendVotesQuery, userID, votes, id, string(e.Kind), peerToDB(e.PeerID), e.Note, treasured).
		Scan(&exists, &left)
	if err != nil {
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

// Move locks the treasury first as well, as Spend and Credit do: a fee or
// a tax spent in the same transaction then keeps the order of the locks.
func (r *WalletRepository) Move(ctx context.Context, fromID, toID uuid.UUID, votes int64, e user.VoteEntry) (int64, error) {
	if err := r.lockTreasury(ctx); err != nil {
		return 0, errs.Wrap("r.lockTreasury", err)
	}

	rows, err := r.conn(ctx).Query(ctx, lockWalletsQuery, fromID, toID)
	if err != nil {
		return 0, errs.Wrap("r.conn.Query", err)
	}

	locked, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, errs.Wrap("pgx.CollectRows", err)
	}

	if len(locked) != 2 {
		return 0, user.ErrNotFound
	}

	// The votes go from page to page: the treasury has no part in it.
	left, err := r.spend(ctx, fromID, votes, user.Entry(e.Kind, toID, e.Note), false)
	if err != nil {
		return 0, errs.Wrap("r.spend", err)
	}

	if _, err = r.credit(ctx, toID, votes, 0, user.Entry(e.Kind, fromID, e.Note), false); err != nil {
		return 0, errs.Wrap("r.credit", err)
	}

	return left, nil
}

// Votes that change are written down; rating alone is not a change of
// votes. With $8 the votes come out of the treasury, which has no bottom.
const creditWalletQuery = `
	WITH credited AS (
		UPDATE users
		SET
			votes = votes + $2,
			rating = rating + $3
		WHERE id = $1
		RETURNING
			votes,
			rating
	), logged AS (
		INSERT INTO vote_operations (
			id,
			user_id,
			kind,
			amount,
			balance,
			peer_id,
			note
		)
		SELECT
			$4,
			$1,
			$5,
			$2,
			votes,
			$6,
			$7
		FROM credited
		WHERE $2 <> 0
	), paid AS (
		UPDATE treasury
		SET votes = votes - $2
		WHERE $8
			AND $2 <> 0
			AND EXISTS (SELECT 1 FROM credited)
	)
	SELECT
		votes,
		rating
	FROM credited
`

// Credit pays the votes out of the treasury.
func (r *WalletRepository) Credit(ctx context.Context, userID uuid.UUID, votes int64, rating int, e user.VoteEntry) (user.Wallet, error) {
	if votes != 0 {
		if err := r.lockTreasury(ctx); err != nil {
			return user.Wallet{}, errs.Wrap("r.lockTreasury", err)
		}
	}

	w, err := r.credit(ctx, userID, votes, rating, e, true)

	return w, errs.Wrap("r.credit", err)
}

// Hold keeps the votes taken out of the treasury.
func (r *WalletRepository) Hold(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error) {
	if err := r.lockTreasury(ctx); err != nil {
		return 0, errs.Wrap("r.lockTreasury", err)
	}

	left, err := r.spend(ctx, userID, votes, e, false)

	return left, errs.Wrap("r.spend", err)
}

// Release gives held votes back past the treasury.
func (r *WalletRepository) Release(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error) {
	if err := r.lockTreasury(ctx); err != nil {
		return 0, errs.Wrap("r.lockTreasury", err)
	}

	w, err := r.credit(ctx, userID, votes, 0, e, false)

	return w.Votes, errs.Wrap("r.credit", err)
}

const depositTreasuryQuery = `
	UPDATE treasury
	SET votes = votes + $1
`

func (r *WalletRepository) Deposit(ctx context.Context, votes int64) error {
	if err := r.lockTreasury(ctx); err != nil {
		return errs.Wrap("r.lockTreasury", err)
	}

	if _, err := r.conn(ctx).Exec(ctx, depositTreasuryQuery, votes); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// credit adds to the wallet of userID, out of the treasury if treasured.
func (r *WalletRepository) credit(ctx context.Context, userID uuid.UUID, votes int64, rating int, e user.VoteEntry, treasured bool) (user.Wallet, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return user.Wallet{}, errs.Wrap("uuid.NewV7", err)
	}

	var w user.Wallet

	err = r.conn(ctx).QueryRow(ctx, creditWalletQuery,
		userID, votes, rating, id, string(e.Kind), peerToDB(e.PeerID), e.Note, treasured).Scan(&w.Votes, &w.Rating)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.Wallet{}, user.ErrNotFound
	}
	if err != nil {
		return user.Wallet{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return w, nil
}

// The limit is checked by the statement that raises the rating; the
// first column tells a user who is gone from one at the limit.
const raiseRatingQuery = `
	WITH raised AS (
		UPDATE users
		SET rating = rating + $2
		WHERE id = $1
			AND rating + $2 <= $3
		RETURNING
			votes,
			rating
	)
	SELECT
		EXISTS (SELECT 1 FROM users WHERE id = $1),
		(SELECT votes FROM raised),
		(SELECT rating FROM raised)
`

func (r *WalletRepository) RaiseRating(ctx context.Context, userID uuid.UUID, percent int) (user.Wallet, error) {
	var (
		exists bool
		votes  *int64
		rating *int
	)

	err := r.conn(ctx).QueryRow(ctx, raiseRatingQuery, userID, percent, user.MaxRating).Scan(&exists, &votes, &rating)
	if err != nil {
		return user.Wallet{}, errs.Wrap("r.conn.QueryRow", err)
	}

	switch {
	case !exists:
		return user.Wallet{}, user.ErrNotFound
	case votes == nil:
		return user.Wallet{}, user.ErrRatingLimit
	default:
		return user.Wallet{Votes: *votes, Rating: *rating}, nil
	}
}

// The vouchers promise what each still gives times how many may still use
// it; the pages hold what is on their accounts.
const treasuryQuery = `
	SELECT
		t.votes,
		coalesce((
			SELECT sum(v.votes * v.usages_left)
			FROM vouchers v
			WHERE v.usages_left > 0
		), 0)::bigint,
		coalesce((
			SELECT sum(u.votes)
			FROM users u
		), 0)::bigint
	FROM treasury t
`

// Treasury locks the treasury, as a change of votes does, and reads it.
func (r *WalletRepository) Treasury(ctx context.Context) (user.Treasury, error) {
	if err := r.lockTreasury(ctx); err != nil {
		return user.Treasury{}, errs.Wrap("r.lockTreasury", err)
	}

	var t user.Treasury
	if err := r.conn(ctx).QueryRow(ctx, treasuryQuery).Scan(&t.Votes, &t.Outstanding, &t.Held); err != nil {
		return user.Treasury{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return t, nil
}

// The bonus of a day is taken once: a later day replaces the one kept.
const takeBonusQuery = `
	INSERT INTO vote_bonuses (
		user_id,
		day
	)
	VALUES ($1, $2)
	ON CONFLICT (user_id) DO UPDATE
	SET day = EXCLUDED.day
	WHERE vote_bonuses.day < EXCLUDED.day
	RETURNING user_id
`

// TakeBonus marks the bonus of day taken by userID; user.ErrBonusTaken.
func (r *WalletRepository) TakeBonus(ctx context.Context, userID uuid.UUID, day time.Time) error {
	var taker uuid.UUID

	err := r.conn(ctx).QueryRow(ctx, takeBonusQuery, userID, day).Scan(&taker)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.ErrBonusTaken
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.QueryRow", err)
	}

	return nil
}

const lastBonusQuery = `
	SELECT day
	FROM vote_bonuses
	WHERE user_id = $1
`

// LastBonus is the day userID took the last bonus on, zero for never.
func (r *WalletRepository) LastBonus(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	var day time.Time

	err := r.conn(ctx).QueryRow(ctx, lastBonusQuery, userID).Scan(&day)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), nil
}

const listVoteOperationsQuery = `
	SELECT
		id,
		kind,
		amount,
		balance,
		peer_id,
		note,
		created_at
	FROM vote_operations
	WHERE user_id = $1
		AND id < $2
		AND ($3 = '' OR ($3 = 'in' AND amount > 0) OR ($3 = 'out' AND amount < 0))
	ORDER BY id DESC
	LIMIT $4
`

func (r *WalletRepository) Operations(ctx context.Context, userID, before uuid.UUID, flow user.VoteFlow, limit int) ([]user.VoteOperation, error) {
	if before == uuid.Nil {
		// Above every UUID: start from the newest operation.
		before = uuid.Max
	}

	rows, err := r.conn(ctx).Query(ctx, listVoteOperationsQuery, userID, before, string(flow), limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ops, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.VoteOperation, error) {
		var (
			op     user.VoteOperation
			kind   string
			peerID *uuid.UUID
		)

		if err := row.Scan(&op.ID, &kind, &op.Amount, &op.Balance, &peerID, &op.Note, &op.At); err != nil {
			return user.VoteOperation{}, err
		}

		op.Kind, op.At = user.VoteKind(kind), op.At.UTC()
		if peerID != nil {
			op.PeerID = *peerID
		}

		return op, nil
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ops, nil
}

const voteTotalsQuery = `
	SELECT
		coalesce(sum(amount) FILTER (WHERE amount > 0), 0),
		coalesce(-sum(amount) FILTER (WHERE amount < 0), 0)
	FROM vote_operations
	WHERE user_id = $1
		AND id >= $2
		AND kind <> 'denomination'
`

func (r *WalletRepository) Totals(ctx context.Context, userID uuid.UUID, since time.Time) (income, expense int64, err error) {
	if err = r.conn(ctx).QueryRow(ctx, voteTotalsQuery, userID, user.OperationsSince(since)).Scan(&income, &expense); err != nil {
		return 0, 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return income, expense, nil
}

// peerToDB is NULL for no peer.
func peerToDB(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
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
