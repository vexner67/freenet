package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// CasinoRepository keeps the casino of the site: the one row of casino.
type CasinoRepository struct {
	pool *pgxpool.Pool
}

func NewCasinoRepository(pool *pgxpool.Pool) *CasinoRepository {
	return &CasinoRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *CasinoRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// casinoPrize is a prize as the column keeps it.
type casinoPrize struct {
	Votes  int64 `json:"votes"`
	Weight int   `json:"weight"`
}

const getCasinoQuery = `
	SELECT
		price,
		prizes
	FROM casino
`

func (r *CasinoRepository) Casino(ctx context.Context) (user.Casino, error) {
	var (
		price  int64
		stored []casinoPrize
	)

	err := r.conn(ctx).QueryRow(ctx, getCasinoQuery).Scan(&price, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DefaultCasino(), nil
	}
	if err != nil {
		return user.Casino{}, errs.Wrap("r.conn.QueryRow", err)
	}

	casino, err := restoreCasino(price, stored)

	return casino, errs.Wrap("restoreCasino", err)
}

const setCasinoQuery = `
	INSERT INTO casino (
		price,
		prizes
	)
	VALUES ($1, $2)
	ON CONFLICT (only_row) DO UPDATE
	SET price = EXCLUDED.price,
		prizes = EXCLUDED.prizes
`

// storedPrizes are the prizes of casino as the column keeps them.
func storedPrizes(casino user.Casino) ([]byte, error) {
	prizes := make([]casinoPrize, 0, len(casino.Prizes()))
	for _, p := range casino.Prizes() {
		prizes = append(prizes, casinoPrize{Votes: p.Votes, Weight: p.Weight})
	}

	stored, err := json.Marshal(prizes)

	return stored, errs.Wrap("json.Marshal", err)
}

// restoreCasino is the casino of a row: price and prizes.
func restoreCasino(price int64, stored []casinoPrize) (user.Casino, error) {
	prizes := make([]user.CasinoPrize, len(stored))
	for i, p := range stored {
		prizes[i] = user.CasinoPrize{Votes: p.Votes, Weight: p.Weight}
	}

	casino, err := user.NewCasino(price, prizes)

	return casino, errs.Wrap("user.NewCasino", err)
}

func (r *CasinoRepository) SetCasino(ctx context.Context, casino user.Casino) error {
	stored, err := storedPrizes(casino)
	if err != nil {
		return errs.Wrap("storedPrizes", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, setCasinoQuery, casino.Price(), stored); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const countCasinoSpinQuery = `
	UPDATE casino
	SET spins = spins + 1,
		spent = spent + $1,
		won = won + $2
`

func (r *CasinoRepository) CountSpin(ctx context.Context, spent, won int64) error {
	if _, err := r.conn(ctx).Exec(ctx, countCasinoSpinQuery, spent, won); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// aimMultiplier is a multiplier of the bets as the column keeps it.
type aimMultiplier struct {
	Multiplier int `json:"multiplier"`
	Chance     int `json:"chance"`
}

// restoreBets are the bets of a row; stored nil for a row without the
// multipliers, which takes the default ones.
func restoreBets(least, most int64, stored []aimMultiplier) (user.CasinoBets, error) {
	aim := user.DefaultAim()
	if stored != nil {
		aim = make([]user.AimMultiplier, len(stored))
		for i, m := range stored {
			aim[i] = user.AimMultiplier{Multiplier: m.Multiplier, Chance: m.Chance}
		}
	}

	bets, err := user.NewCasinoBets(least, most, aim)

	return bets, errs.Wrap("user.NewCasinoBets", err)
}

// storedAim are the multipliers of bets as the column keeps them.
func storedAim(bets user.CasinoBets) ([]byte, error) {
	aim := make([]aimMultiplier, 0, len(bets.Aim()))
	for _, m := range bets.Aim() {
		aim = append(aim, aimMultiplier{Multiplier: m.Multiplier, Chance: m.Chance})
	}

	stored, err := json.Marshal(aim)

	return stored, errs.Wrap("json.Marshal", err)
}

const getCasinoBetsQuery = `
	SELECT
		bet_least,
		bet_most,
		bet_aim
	FROM casino
`

func (r *CasinoRepository) Bets(ctx context.Context) (user.CasinoBets, error) {
	var (
		least, most int64
		aim         []aimMultiplier
	)

	err := r.conn(ctx).QueryRow(ctx, getCasinoBetsQuery).Scan(&least, &most, &aim)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DefaultCasinoBets(), nil
	}
	if err != nil {
		return user.CasinoBets{}, errs.Wrap("r.conn.QueryRow", err)
	}

	bets, err := restoreBets(least, most, aim)

	return bets, errs.Wrap("restoreBets", err)
}

// Without the row the bets come with the default casino, as Casino reads
// it then, so that they are kept all the same.
const setCasinoBetsQuery = `
	INSERT INTO casino (
		price,
		prizes,
		bet_least,
		bet_most,
		bet_aim
	)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (only_row) DO UPDATE
	SET bet_least = EXCLUDED.bet_least,
		bet_most = EXCLUDED.bet_most,
		bet_aim = EXCLUDED.bet_aim
`

func (r *CasinoRepository) SetBets(ctx context.Context, bets user.CasinoBets) error {
	casino := user.DefaultCasino()

	prizes, err := storedPrizes(casino)
	if err != nil {
		return errs.Wrap("storedPrizes", err)
	}

	aim, err := storedAim(bets)
	if err != nil {
		return errs.Wrap("storedAim", err)
	}

	_, err = r.conn(ctx).Exec(ctx, setCasinoBetsQuery, casino.Price(), prizes, bets.Least(), bets.Most(), aim)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const countCasinoBetQuery = `
	UPDATE casino
	SET bets = bets + 1,
		bet_spent = bet_spent + $1,
		bet_won = bet_won + $2
`

func (r *CasinoRepository) CountBet(ctx context.Context, spent, won int64) error {
	if _, err := r.conn(ctx).Exec(ctx, countCasinoBetQuery, spent, won); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getCasinoSettingsQuery = `
	SELECT
		price,
		prizes,
		bet_least,
		bet_most,
		bet_aim,
		spins,
		spent,
		won,
		bets,
		bet_spent,
		bet_won
	FROM casino
`

func (r *CasinoRepository) Settings(ctx context.Context) (service.CasinoSettings, error) {
	var (
		price       int64
		stored      []casinoPrize
		least, most int64
		aim         []aimMultiplier
		stats       user.CasinoStats
	)

	err := r.conn(ctx).QueryRow(ctx, getCasinoSettingsQuery).Scan(
		&price,
		&stored,
		&least,
		&most,
		&aim,
		&stats.Spins,
		&stats.Spent,
		&stats.Won,
		&stats.Wagers,
		&stats.WagerSpent,
		&stats.WagerWon,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.CasinoSettings{Casino: user.DefaultCasino(), Bets: user.DefaultCasinoBets()}, nil
	}
	if err != nil {
		return service.CasinoSettings{}, errs.Wrap("r.conn.QueryRow", err)
	}

	casino, err := restoreCasino(price, stored)
	if err != nil {
		return service.CasinoSettings{}, errs.Wrap("restoreCasino", err)
	}

	bets, err := restoreBets(least, most, aim)
	if err != nil {
		return service.CasinoSettings{}, errs.Wrap("restoreBets", err)
	}

	return service.CasinoSettings{Casino: casino, Bets: bets, Stats: stats}, nil
}
