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

// MineRepository keeps the mine of the site, the one row of mine, and its
// miners.
type MineRepository struct {
	pool *pgxpool.Pool
}

func NewMineRepository(pool *pgxpool.Pool) *MineRepository {
	return &MineRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *MineRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// storedVein is a vein as the settings keep it.
type storedVein struct {
	Ore        string `json:"ore"`
	Experience int64  `json:"experience"`
	Least      int64  `json:"least"`
	Most       int64  `json:"most"`
	Price      int64  `json:"price"`
	Gain       int64  `json:"gain"`
	Energy     int    `json:"energy"`
}

// storedEvents are the events as the settings keep them.
type storedEvents struct {
	Nugget      int   `json:"nugget"`
	NuggetTimes int   `json:"nuggetTimes"`
	Chest       int   `json:"chest"`
	ChestLeast  int64 `json:"chestLeast"`
	ChestMost   int64 `json:"chestMost"`
	Collapse    int   `json:"collapse"`
	Drink       int   `json:"drink"`
	DrinkEnergy int   `json:"drinkEnergy"`
}

// storedMine is the mine as the column keeps it.
type storedMine struct {
	Veins  []storedVein `json:"veins"`
	Energy int          `json:"energy"`
	Regen  int64        `json:"regenSeconds"`
	Events storedEvents `json:"events"`
}

func storeMine(mine user.Mine) ([]byte, error) {
	stored := storedMine{Energy: mine.Energy(), Regen: int64(mine.Regen() / time.Second), Events: storedEvents(mine.Events())}
	for _, v := range mine.Veins() {
		stored.Veins = append(stored.Veins, storedVein{
			Ore: v.Ore.String(), Experience: v.Experience, Least: v.Least, Most: v.Most,
			Price: v.Price, Gain: v.Gain, Energy: v.Energy,
		})
	}

	data, err := json.Marshal(stored)

	return data, errs.Wrap("json.Marshal", err)
}

func restoreMine(stored storedMine) (user.Mine, error) {
	veins := make([]user.OreVein, len(stored.Veins))
	for i, v := range stored.Veins {
		veins[i] = user.OreVein{
			Ore: user.Ore(v.Ore), Experience: v.Experience, Least: v.Least, Most: v.Most,
			Price: v.Price, Gain: v.Gain, Energy: v.Energy,
		}
	}

	mine, err := user.NewMine(veins, stored.Energy, time.Duration(stored.Regen)*time.Second, user.MineEvents(stored.Events))

	return mine, errs.Wrap("user.NewMine", err)
}

const getMineQuery = `
	SELECT settings
	FROM mine
`

func (r *MineRepository) Mine(ctx context.Context) (user.Mine, error) {
	var stored *storedMine

	err := r.conn(ctx).QueryRow(ctx, getMineQuery).Scan(&stored)
	if err != nil {
		return user.Mine{}, errs.Wrap("r.conn.QueryRow", err)
	}
	if stored == nil {
		return user.DefaultMine(), nil
	}

	mine, err := restoreMine(*stored)

	return mine, errs.Wrap("restoreMine", err)
}

const setMineQuery = `
	INSERT INTO mine (settings)
	VALUES ($1)
	ON CONFLICT (only_row) DO UPDATE
	SET settings = EXCLUDED.settings
`

func (r *MineRepository) SetMine(ctx context.Context, mine user.Mine) error {
	stored, err := storeMine(mine)
	if err != nil {
		return errs.Wrap("storeMine", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, setMineQuery, stored); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// lockMinerQuery reads the miner addMinerQuery made sure of and locks
// it.
const lockMinerQuery = `
	SELECT
		energy,
		counted_at,
		experience,
		backpack,
		digs,
		earned
	FROM miners
	WHERE user_id = $1
	FOR UPDATE
`

func (r *MineRepository) Miner(ctx context.Context, userID uuid.UUID, fresh user.Miner) (user.Miner, error) {
	_, err := r.conn(ctx).Exec(ctx, addMinerQuery, userID, fresh.Energy(), fresh.CountedAt())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.Miner{}, user.ErrNotFound
	}
	if err != nil {
		return user.Miner{}, errs.Wrap("r.conn.Exec", err)
	}

	var (
		energy                   int
		countedAt                time.Time
		experience, digs, earned int64
		backpack                 map[string]int64
	)

	err = r.conn(ctx).QueryRow(ctx, lockMinerQuery, userID).Scan(&energy, &countedAt, &experience, &backpack, &digs, &earned)
	if err != nil {
		return user.Miner{}, errs.Wrap("r.conn.QueryRow", err)
	}

	ores := make(map[user.Ore]int64, len(backpack))
	for o, units := range backpack {
		ores[user.Ore(o)] = units
	}

	return user.RestoreMiner(energy, countedAt, experience, ores, digs, earned), nil
}

const addMinerQuery = `
	INSERT INTO miners (
		user_id,
		energy,
		counted_at
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (user_id) DO NOTHING
`

const saveMinerQuery = `
	UPDATE miners
	SET energy = $2,
		counted_at = $3,
		experience = $4,
		backpack = $5,
		digs = $6,
		earned = $7
	WHERE user_id = $1
`

func (r *MineRepository) SaveMiner(ctx context.Context, userID uuid.UUID, m user.Miner) error {
	backpack := map[string]int64{}
	for o, units := range m.Backpack() {
		backpack[o.String()] = units
	}

	tag, err := r.conn(ctx).Exec(ctx, saveMinerQuery, userID, m.Energy(), m.CountedAt(), m.Experience(), backpack, m.Digs(), m.Earned())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}

const countMineDigQuery = `
	UPDATE mine
	SET digs = digs + 1,
		chests = chests + $1
`

func (r *MineRepository) CountDig(ctx context.Context, chest int64) error {
	if _, err := r.conn(ctx).Exec(ctx, countMineDigQuery, chest); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const countMineSaleQuery = `
	UPDATE mine
	SET sold = sold + $1
`

func (r *MineRepository) CountSale(ctx context.Context, votes int64) error {
	if _, err := r.conn(ctx).Exec(ctx, countMineSaleQuery, votes); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const mineStatsQuery = `
	SELECT
		digs,
		sold,
		chests,
		(SELECT count(*) FROM miners WHERE digs > 0)
	FROM mine
`

func (r *MineRepository) Stats(ctx context.Context) (user.MineStats, error) {
	var stats user.MineStats

	err := r.conn(ctx).QueryRow(ctx, mineStatsQuery).Scan(&stats.Digs, &stats.Sold, &stats.Chests, &stats.Miners)

	return stats, errs.Wrap("r.conn.QueryRow", err)
}
