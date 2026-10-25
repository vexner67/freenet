package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// BusinessRepository keeps the rules of the businesses, the one row of
// business, and the businesses of the pages.
type BusinessRepository struct {
	pool *pgxpool.Pool
}

func NewBusinessRepository(pool *pgxpool.Pool) *BusinessRepository {
	return &BusinessRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *BusinessRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// storedPlan is a plan as the settings keep it.
type storedPlan struct {
	Kind    user.BusinessKind `json:"kind"`
	Price   int64             `json:"price"`
	Upgrade int64             `json:"upgrade"`
	Levels  int               `json:"levels"`
	Demand  int64             `json:"demand"`
	Fair    int64             `json:"fair"`
	Cost    int64             `json:"cost"`
	Storage int64             `json:"storage"`
	Serve   int64             `json:"serve"`
	Staff   int               `json:"staff"`
	Wage    int64             `json:"wage"`
	Rent    int64             `json:"rent"`
	Wear    int               `json:"wear"`
	Repair  int64             `json:"repair"`
	Till    int64             `json:"till"`
	Debt    int64             `json:"debt"`
	AdPrice int64             `json:"adPrice"`
	AdHours int               `json:"adHours"`
	AdBoost int               `json:"adBoost"`
}

// storedRules are the rules as the column keeps them.
type storedRules struct {
	Plans []storedPlan `json:"plans"`
	Tax   int          `json:"tax"`
}

func storeRules(rules user.BusinessRules) ([]byte, error) {
	stored := storedRules{Tax: rules.Tax()}
	for _, p := range rules.Plans() {
		sp := storedPlan(p)
		stored.Plans = append(stored.Plans, sp)
	}

	data, err := json.Marshal(stored)

	return data, errs.Wrap("json.Marshal", err)
}

func restoreRules(stored storedRules) (user.BusinessRules, error) {
	plans := make([]user.BusinessPlan, len(stored.Plans))
	for i, p := range stored.Plans {
		plans[i] = user.BusinessPlan(p)
	}

	rules, err := user.NewBusinessRules(plans, stored.Tax)

	return rules, errs.Wrap("user.NewBusinessRules", err)
}

const getBusinessRulesQuery = `
	SELECT settings
	FROM business
`

func (r *BusinessRepository) Rules(ctx context.Context) (user.BusinessRules, error) {
	var stored *storedRules

	err := r.conn(ctx).QueryRow(ctx, getBusinessRulesQuery).Scan(&stored)
	if err != nil {
		return user.BusinessRules{}, errs.Wrap("r.conn.QueryRow", err)
	}
	if stored == nil {
		return user.DefaultBusinessRules(), nil
	}

	rules, err := restoreRules(*stored)

	return rules, errs.Wrap("restoreRules", err)
}

const setBusinessRulesQuery = `
	INSERT INTO business (settings)
	VALUES ($1)
	ON CONFLICT (only_row) DO UPDATE
	SET settings = EXCLUDED.settings
`

func (r *BusinessRepository) SetRules(ctx context.Context, rules user.BusinessRules) error {
	stored, err := storeRules(rules)
	if err != nil {
		return errs.Wrap("storeRules", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, setBusinessRulesQuery, stored); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const businessColumns = `
		kind,
		level,
		stock,
		price,
		staff,
		condition,
		cash,
		ad_until,
		counted_at,
		bankrupt,
		invested,
		collected,
		perks`

func scanBusiness(row pgx.Row) (user.Business, error) {
	var (
		s     user.BusinessState
		kind  string
		perks map[string]int
	)

	err := row.Scan(&kind, &s.Level, &s.Stock, &s.Price, &s.Staff, &s.Condition, &s.Cash,
		&s.AdUntil, &s.CountedAt, &s.Bankrupt, &s.Invested, &s.Collected, &perks)
	if err != nil {
		return user.Business{}, err
	}

	s.Kind = user.BusinessKind(kind)
	s.Perks = make(map[user.BusinessPerk]int, len(perks))
	for p, tier := range perks {
		s.Perks[user.BusinessPerk(p)] = tier
	}

	return user.RestoreBusiness(s), nil
}

const listBusinessesQuery = `
	SELECT` + businessColumns + `
	FROM businesses
	WHERE user_id = $1
	ORDER BY array_position(ARRAY['kiosk', 'cafe', 'carwash', 'factory'], kind)
`

func (r *BusinessRepository) Businesses(ctx context.Context, userID uuid.UUID) ([]user.Business, error) {
	rows, err := r.conn(ctx).Query(ctx, listBusinessesQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var owned []user.Business
	for rows.Next() {
		b, err := scanBusiness(rows)
		if err != nil {
			return nil, errs.Wrap("scanBusiness", err)
		}

		owned = append(owned, b)
	}

	return owned, errs.Wrap("rows.Err", rows.Err())
}

const lockBusinessQuery = `
	SELECT` + businessColumns + `
	FROM businesses
	WHERE user_id = $1
		AND kind = $2
	FOR UPDATE
`

func (r *BusinessRepository) Business(ctx context.Context, userID uuid.UUID, kind user.BusinessKind) (user.Business, error) {
	b, err := scanBusiness(r.conn(ctx).QueryRow(ctx, lockBusinessQuery, userID, kind.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.Business{}, user.ErrBusinessNotFound
	}
	if err != nil {
		return user.Business{}, errs.Wrap("scanBusiness", err)
	}

	return b, nil
}

const addBusinessQuery = `
	INSERT INTO businesses (
		user_id,` + businessColumns + `
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	ON CONFLICT (user_id, kind) DO NOTHING
`

func businessArgs(userID uuid.UUID, b user.Business) []any {
	s := b.State()

	perks := make(map[string]int, len(s.Perks))
	for p, tier := range s.Perks {
		perks[p.String()] = tier
	}

	return []any{userID, s.Kind.String(), s.Level, s.Stock, s.Price, s.Staff, s.Condition, s.Cash,
		s.AdUntil, s.CountedAt, s.Bankrupt, s.Invested, s.Collected, perks}
}

func (r *BusinessRepository) AddBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error {
	tag, err := r.conn(ctx).Exec(ctx, addBusinessQuery, businessArgs(userID, b)...)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrBusinessOwned
	}

	return nil
}

const saveBusinessQuery = `
	UPDATE businesses
	SET level = $3,
		stock = $4,
		price = $5,
		staff = $6,
		condition = $7,
		cash = $8,
		ad_until = $9,
		counted_at = $10,
		bankrupt = $11,
		invested = $12,
		collected = $13,
		perks = $14
	WHERE user_id = $1
		AND kind = $2
`

func (r *BusinessRepository) SaveBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error {
	tag, err := r.conn(ctx).Exec(ctx, saveBusinessQuery, businessArgs(userID, b)...)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrBusinessNotFound
	}

	return nil
}

const deleteBusinessQuery = `
	DELETE FROM businesses
	WHERE user_id = $1
		AND kind = $2
`

func (r *BusinessRepository) DeleteBusiness(ctx context.Context, userID uuid.UUID, kind user.BusinessKind) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteBusinessQuery, userID, kind.String()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const countBusinessQuery = `
	UPDATE business
	SET spent = spent + $1,
		paid = paid + $2
`

func (r *BusinessRepository) Count(ctx context.Context, spent, paid int64) error {
	if _, err := r.conn(ctx).Exec(ctx, countBusinessQuery, spent, paid); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const businessStatsQuery = `
	SELECT
		(SELECT count(*) FROM businesses),
		spent,
		paid
	FROM business
`

func (r *BusinessRepository) Stats(ctx context.Context) (user.BusinessStats, error) {
	var stats user.BusinessStats

	err := r.conn(ctx).QueryRow(ctx, businessStatsQuery).Scan(&stats.Businesses, &stats.Spent, &stats.Paid)

	return stats, errs.Wrap("r.conn.QueryRow", err)
}
