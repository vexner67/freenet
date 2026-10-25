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
	Demand  int               `json:"demand"`
	Staff   int               `json:"staff"`
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
	Wages int          `json:"wages"`
	Goods int          `json:"goods"`
}

func storeRules(rules user.BusinessRules) ([]byte, error) {
	stored := storedRules{Tax: rules.Tax(), Wages: rules.Wages(), Goods: rules.Goods()}
	for _, p := range rules.Plans() {
		stored.Plans = append(stored.Plans, storedPlan(p))
	}

	data, err := json.Marshal(stored)

	return data, errs.Wrap("json.Marshal", err)
}

func restoreRules(stored storedRules) (user.BusinessRules, error) {
	plans := make([]user.BusinessPlan, len(stored.Plans))
	for i, p := range stored.Plans {
		plans[i] = user.BusinessPlan(p)
	}

	rules, err := user.NewBusinessRules(plans, stored.Tax, stored.Wages, stored.Goods)

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

func scanBusiness(row pgx.Row) (user.Business, error) {
	var stored storedBusiness
	if err := row.Scan(&stored); err != nil {
		return user.Business{}, err
	}

	return user.RestoreBusiness(stored.restore()), nil
}

const listBusinessesQuery = `
	SELECT state
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
	SELECT state
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
		user_id,
		kind,
		state
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (user_id, kind) DO NOTHING
`

func (r *BusinessRepository) AddBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error {
	tag, err := r.conn(ctx).Exec(ctx, addBusinessQuery, userID, b.Kind().String(), storeBusiness(b.State()))

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
	SET state = $3
	WHERE user_id = $1
		AND kind = $2
`

func (r *BusinessRepository) SaveBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error {
	tag, err := r.conn(ctx).Exec(ctx, saveBusinessQuery, userID, b.Kind().String(), storeBusiness(b.State()))
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
