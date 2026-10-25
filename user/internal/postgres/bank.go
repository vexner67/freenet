package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// BankRepository keeps the bank of the site, the one row of bank, and the
// accounts of pages.
type BankRepository struct {
	pool *pgxpool.Pool
}

func NewBankRepository(pool *pgxpool.Pool) *BankRepository {
	return &BankRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *BankRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// storedBank is the bank as the column keeps it.
type storedBank struct {
	DepositRate int   `json:"depositRate"`
	DepositMost int64 `json:"depositMost"`
	LoanRate    int   `json:"loanRate"`
	LoanMost    int64 `json:"loanMost"`
	LoanDays    int   `json:"loanDays"`
	PenaltyRate int   `json:"penaltyRate"`
}

const getBankQuery = `
	SELECT settings
	FROM bank
`

func (r *BankRepository) Bank(ctx context.Context) (user.Bank, error) {
	var stored *storedBank

	err := r.conn(ctx).QueryRow(ctx, getBankQuery).Scan(&stored)
	if err != nil {
		return user.Bank{}, errs.Wrap("r.conn.QueryRow", err)
	}
	if stored == nil {
		return user.DefaultBank, nil
	}

	b, err := user.NewBank(user.Bank(*stored))

	return b, errs.Wrap("user.NewBank", err)
}

const setBankQuery = `
	INSERT INTO bank (settings)
	VALUES ($1)
	ON CONFLICT (only_row) DO UPDATE
	SET settings = EXCLUDED.settings
`

func (r *BankRepository) SetBank(ctx context.Context, b user.Bank) error {
	stored, err := json.Marshal(storedBank(b))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, setBankQuery, stored); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const addBankAccountQuery = `
	INSERT INTO bank_accounts (user_id)
	VALUES ($1)
	ON CONFLICT (user_id) DO NOTHING
`

// lockBankAccountQuery reads the account addBankAccountQuery made sure
// of and locks it.
const lockBankAccountQuery = `
	SELECT
		deposit,
		deposit_at,
		debt,
		debt_at,
		borrowed,
		due_at
	FROM bank_accounts
	WHERE user_id = $1
	FOR UPDATE
`

func (r *BankRepository) Account(ctx context.Context, userID uuid.UUID) (user.BankAccount, error) {
	_, err := r.conn(ctx).Exec(ctx, addBankAccountQuery, userID)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.BankAccount{}, user.ErrNotFound
	}
	if err != nil {
		return user.BankAccount{}, errs.Wrap("r.conn.Exec", err)
	}

	var (
		deposit, debt, borrowed  int64
		depositAt, debtAt, dueAt time.Time
	)

	err = r.conn(ctx).QueryRow(ctx, lockBankAccountQuery, userID).Scan(&deposit, &depositAt, &debt, &debtAt, &borrowed, &dueAt)
	if err != nil {
		return user.BankAccount{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return user.RestoreBankAccount(deposit, depositAt.UTC(), debt, debtAt.UTC(), borrowed, dueAt.UTC()), nil
}

const saveBankAccountQuery = `
	UPDATE bank_accounts
	SET deposit = $2,
		deposit_at = $3,
		debt = $4,
		debt_at = $5,
		borrowed = $6,
		due_at = $7
	WHERE user_id = $1
`

func (r *BankRepository) SaveAccount(ctx context.Context, userID uuid.UUID, a user.BankAccount) error {
	tag, err := r.conn(ctx).Exec(ctx, saveBankAccountQuery, userID, a.Deposit(), a.DepositAt(), a.Debt(), a.DebtAt(), a.Borrowed(), a.DueAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}

// overdueQuery skips the accounts another replica collects now.
const overdueQuery = `
	SELECT user_id
	FROM bank_accounts
	WHERE debt > 0
		AND due_at < $1
	ORDER BY due_at
	LIMIT $2
	FOR UPDATE SKIP LOCKED
`

func (r *BankRepository) Overdue(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, overdueQuery, now, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		ids = append(ids, id)
	}

	return ids, errs.Wrap("rows.Err", rows.Err())
}

const bankStatsQuery = `
	SELECT
		coalesce(sum(deposit), 0),
		count(*) FILTER (WHERE deposit > 0),
		coalesce(sum(debt), 0),
		count(*) FILTER (WHERE debt > 0)
	FROM bank_accounts
`

func (r *BankRepository) Stats(ctx context.Context) (user.BankStats, error) {
	var stats user.BankStats

	err := r.conn(ctx).QueryRow(ctx, bankStatsQuery).Scan(&stats.Deposits, &stats.Depositors, &stats.Debts, &stats.Debtors)

	return stats, errs.Wrap("r.conn.QueryRow", err)
}
