package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// gamesLock separates a closing snapshot from game requests and jobs
// across all replicas.
const gamesLock int64 = 678_000_431_023_180

type GamesClosingRepository struct {
	pool *pgxpool.Pool
	tx   *pgpool.TxManager
}

func NewGamesClosingRepository(pool *pgxpool.Pool) *GamesClosingRepository {
	return &GamesClosingRepository{pool: pool, tx: pgpool.NewTxManager(pool)}
}

func (r *GamesClosingRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

func (r *GamesClosingRepository) Lock(ctx context.Context) error {
	_, err := r.conn(ctx).Exec(ctx, "SELECT pg_advisory_xact_lock($1)", gamesLock)
	return errs.Wrap("r.conn.Exec", err)
}

const gamesFrozenQuery = `SELECT EXISTS (SELECT 1 FROM games_closure)`

func (r *GamesClosingRepository) frozen(ctx context.Context) (bool, error) {
	var frozen bool
	err := r.conn(ctx).QueryRow(ctx, gamesFrozenQuery).Scan(&frozen)
	return frozen, errs.Wrap("r.conn.QueryRow.Scan", err)
}

func (r *GamesClosingRepository) WhileOpen(ctx context.Context, fn func(context.Context) error) error {
	err := r.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := r.conn(ctx).Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", gamesLock); err != nil {
			return errs.Wrap("r.conn.Exec", err)
		}
		frozen, err := r.frozen(ctx)
		if err != nil {
			return errs.Wrap("r.frozen", err)
		}
		if frozen {
			return user.ErrGamesClosed
		}

		return fn(ctx)
	})

	return errs.Wrap("r.tx.WithinTx", err)
}

const closingPlanQuery = `
	SELECT user_id, amounts, total
	FROM games_closing
	ORDER BY total DESC, user_id
`

func (r *GamesClosingRepository) Plan(ctx context.Context) (service.ClosingReport, bool, error) {
	frozen, err := r.frozen(ctx)
	if err != nil || !frozen {
		return service.ClosingReport{}, frozen, errs.Wrap("r.frozen", err)
	}

	accounts, err := closingRows(ctx, r.conn(ctx), closingPlanQuery, func(row pgx.CollectableRow) (service.ClosingAccount, error) {
		var a service.ClosingAccount
		err := row.Scan(&a.UserID, &a.Amounts, &a.Total)
		return a, errs.Wrap("row.Scan", err)
	})
	if err != nil {
		return service.ClosingReport{}, true, errs.Wrap("closingRows", err)
	}
	report := service.ClosingReport{Accounts: accounts, ByReason: make(map[string]int64)}
	for _, a := range accounts {
		report.Total += a.Total
		for reason, amount := range a.Amounts {
			report.ByReason[reason] += amount
		}
	}

	return report, true, nil
}

func (r *GamesClosingRepository) SavePlan(ctx context.Context, at time.Time, report service.ClosingReport) error {
	if _, err := r.conn(ctx).Exec(ctx, "INSERT INTO games_closure (frozen_at) VALUES ($1)", at); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	for _, a := range report.Accounts {
		_, err := r.conn(ctx).Exec(ctx, "INSERT INTO games_closing (user_id, amounts, total) VALUES ($1, $2, $3)", a.UserID, a.Amounts, a.Total)
		if err != nil {
			return errs.Wrap("r.conn.Exec", err)
		}
	}

	return nil
}

func (r *GamesClosingRepository) Paid(ctx context.Context, id uuid.UUID) (bool, error) {
	var paid bool
	err := r.conn(ctx).QueryRow(ctx, "SELECT paid_at IS NOT NULL FROM games_closing WHERE user_id = $1 FOR UPDATE", id).Scan(&paid)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return paid, errs.Wrap("r.conn.QueryRow.Scan", err)
}

func (r *GamesClosingRepository) MarkPaid(ctx context.Context, id uuid.UUID) error {
	_, err := r.conn(ctx).Exec(ctx, "UPDATE games_closing SET paid_at = now() WHERE user_id = $1", id)
	return errs.Wrap("r.conn.Exec", err)
}

func (r *GamesClosingRepository) Existing(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	ids, err := closingRows(ctx, r.conn(ctx), "SELECT id FROM users WHERE id = ANY($1)", pgx.RowTo[uuid.UUID], ids)
	return ids, errs.Wrap("closingRows", err)
}

func closingRows[T any](ctx context.Context, conn pgpool.Querier, query string, scan pgx.RowToFunc[T], args ...any) ([]T, error) {
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("conn.Query", err)
	}

	got, err := pgx.CollectRows(rows, scan)
	return got, errs.Wrap("pgx.CollectRows", err)
}

func (r *GamesClosingRepository) Assets(ctx context.Context, at time.Time) (user.ClosingAssets, error) {
	assets, err := r.financialAssets(ctx, at)
	if err != nil {
		return user.ClosingAssets{}, errs.Wrap("r.financialAssets", err)
	}
	if err = r.inventory(ctx, &assets); err != nil {
		return user.ClosingAssets{}, errs.Wrap("r.inventory", err)
	}

	return assets, nil
}

func (r *GamesClosingRepository) financialAssets(ctx context.Context, at time.Time) (user.ClosingAssets, error) {
	firms := NewFirmRepository(r.pool)
	city, err := firms.City(ctx, false)
	if err != nil {
		return user.ClosingAssets{}, errs.Wrap("firms.City", err)
	}
	rules, err := firms.Rules(ctx)
	if err != nil {
		return user.ClosingAssets{}, errs.Wrap("firms.Rules", err)
	}

	a := user.ClosingAssets{City: city, Rules: rules}
	a.Tables, err = closingRows(ctx, r.conn(ctx), getHoldemTableQuery, scanClosingTable)
	if err != nil {
		return a, errs.Wrap("closingRows", err)
	}
	a.Stakes, err = closingRows(ctx, r.conn(ctx), closingStakesQuery, pgx.RowToStructByPos[user.Payout])
	if err != nil {
		return a, errs.Wrap("closingRows", err)
	}
	a.Poker, err = closingRows(ctx, r.conn(ctx), "SELECT user_id, ante FROM poker_hands WHERE state = 'dealt'", pgx.RowToStructByPos[user.Payout])
	if err != nil {
		return a, errs.Wrap("closingRows", err)
	}
	a.Rockets, err = closingRows(ctx, r.conn(ctx), closingRocketsQuery, scanClosingRocket, at.Add(-time.Hour))

	return a, errs.Wrap("closingRows", err)
}

const closingStakesQuery = `
	SELECT s.user_id, s.amount
	FROM bet_stakes s
	JOIN bet_events e ON e.id = s.event_id
	WHERE e.state IN ('open', 'closed')
`

const closingRocketsQuery = `
	SELECT id, user_id, amount, crash, started_at
	FROM rocket_rounds
	WHERE cashed IS NULL AND auto IS NULL AND started_at > $1
`

func scanClosingTable(row pgx.CollectableRow) (*user.HoldemTable, error) {
	var id uuid.UUID
	var raw []byte
	if err := row.Scan(&id, &raw); err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}
	table, err := restoreHoldem(id, raw)
	return table, errs.Wrap("restoreHoldem", err)
}

func scanClosingRocket(row pgx.CollectableRow) (*user.RocketRound, error) {
	var id, owner uuid.UUID
	var amount int64
	var crash int
	var at time.Time
	if err := row.Scan(&id, &owner, &amount, &crash, &at); err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}
	return user.RestoreRocketRound(id, owner, amount, crash, 0, at, 0, 0), nil
}

func (r *GamesClosingRepository) inventory(ctx context.Context, a *user.ClosingAssets) error {
	var err error
	a.Mine, err = NewMineRepository(r.pool).Mine(ctx)
	if err != nil {
		return errs.Wrap("MineRepository.Mine", err)
	}
	a.Shop, err = NewPetRepository(r.pool).Shop(ctx)
	if err != nil {
		return errs.Wrap("PetRepository.Shop", err)
	}
	a.Miners, err = closingRows(ctx, r.conn(ctx), "SELECT user_id, backpack FROM miners", pgx.RowToStructByPos[user.ClosingMiner])
	if err != nil {
		return errs.Wrap("closingRows", err)
	}
	a.Pets, err = closingRows(ctx, r.conn(ctx), closingPetsQuery, scanClosingPet)
	if err != nil {
		return errs.Wrap("closingRows", err)
	}
	tournaments, err := closingRows(ctx, r.conn(ctx), "SELECT state FROM pet_tournaments", pgx.RowTo[user.PetTournamentState])
	if err != nil {
		return errs.Wrap("closingRows", err)
	}
	for _, tournament := range tournaments {
		if tournament.RanAt.IsZero() {
			for _, bet := range tournament.Bets {
				a.Stakes = append(a.Stakes, user.Payout{UserID: bet.UserID, Votes: bet.Votes})
			}
		}
	}

	return nil
}

const closingPetsQuery = `
	SELECT user_id, kind, wardrobe
	FROM pets
	UNION ALL
	SELECT user_id, drop_kind, ARRAY[]::text[]
	FROM pet_keepers
	WHERE drop_kind IS NOT NULL
`

func scanClosingPet(row pgx.CollectableRow) (user.ClosingPet, error) {
	var pet user.ClosingPet
	var names []string
	if err := row.Scan(&pet.UserID, &pet.Kind, &names); err != nil {
		return user.ClosingPet{}, errs.Wrap("row.Scan", err)
	}
	pet.Wardrobe = petItems(names)
	return pet, nil
}
