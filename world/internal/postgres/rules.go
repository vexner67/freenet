package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/world/internal/world"
)

// RulesRepository keeps the rules of «Мир» (world_rules).
type RulesRepository struct {
	pool *pgxpool.Pool
}

func NewRulesRepository(pool *pgxpool.Pool) *RulesRepository {
	return &RulesRepository{
		pool: pool,
	}
}

type storedRules struct {
	FoundCost       int64 `json:"foundCost"`
	FoundTreasury   int64 `json:"foundTreasury"`
	FoundLands      int   `json:"foundLands"`
	CapitalDistance int   `json:"capitalDistance"`
	ProtectionDays  int   `json:"protectionDays"`
	ColonizeCost    int64 `json:"colonizeCost"`
	ColonizeHours   int   `json:"colonizeHours"`
	Income          int64 `json:"income"`
}

const rulesQuery = `SELECT rules FROM world_rules`

// Rules are world.DefaultRules without the row; rules stored before a
// rule was added keep its default.
func (r *RulesRepository) Rules(ctx context.Context) (world.Rules, error) {
	s := storedRules(world.DefaultRules)
	var data []byte
	err := r.pool.QueryRow(ctx, rulesQuery).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return world.DefaultRules, nil
	}
	if err != nil {
		return world.Rules{}, errs.Wrap("row.Scan", err)
	}

	if err = json.Unmarshal(data, &s); err != nil {
		return world.Rules{}, errs.Wrap("json.Unmarshal", err)
	}

	return world.Rules(s), nil
}

const setRulesQuery = `
	INSERT INTO world_rules (
		id,
		rules
	)
	VALUES (true, $1)
	ON CONFLICT (id) DO UPDATE SET
		rules = EXCLUDED.rules
`

func (r *RulesRepository) SetRules(ctx context.Context, rules world.Rules) error {
	data, err := json.Marshal(storedRules(rules))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.pool.Exec(ctx, setRulesQuery, data)

	return errs.Wrap("r.pool.Exec", err)
}
