package postgres

import (
	"context"
	"encoding/json"
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

// PokerRepository keeps "Покер": the one row of poker and the table
// poker_hands.
type PokerRepository struct {
	pool *pgxpool.Pool
}

func NewPokerRepository(pool *pgxpool.Pool) *PokerRepository {
	return &PokerRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *PokerRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// storedPoker is the table as the column keeps it.
type storedPoker struct {
	Least int64 `json:"least"`
	Most  int64 `json:"most"`
	Pays  []int `json:"pays"`
}

const getPokerQuery = `
	SELECT settings
	FROM poker
`

func (r *PokerRepository) Poker(ctx context.Context) (user.Poker, error) {
	var stored *storedPoker

	err := r.conn(ctx).QueryRow(ctx, getPokerQuery).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.DefaultPoker(), nil
	}
	if err != nil {
		return user.Poker{}, errs.Wrap("r.conn.QueryRow", err)
	}
	if stored == nil {
		return user.DefaultPoker(), nil
	}

	var pays [user.PokerPays]int
	copy(pays[:], stored.Pays)

	p, err := user.NewPoker(stored.Least, stored.Most, pays)

	return p, errs.Wrap("user.NewPoker", err)
}

const setPokerQuery = `
	INSERT INTO poker (settings)
	VALUES ($1)
	ON CONFLICT (only_row) DO UPDATE
	SET settings = EXCLUDED.settings
`

func (r *PokerRepository) SetPoker(ctx context.Context, p user.Poker) error {
	pays := p.Pays()

	stored, err := json.Marshal(storedPoker{Least: p.Least(), Most: p.Most(), Pays: pays[:]})
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, setPokerQuery, stored); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getPokerStatsQuery = `
	SELECT
		hands,
		spent,
		won
	FROM poker
`

func (r *PokerRepository) Stats(ctx context.Context) (user.PokerStats, error) {
	var stats user.PokerStats

	err := r.conn(ctx).QueryRow(ctx, getPokerStatsQuery).Scan(&stats.Hands, &stats.Spent, &stats.Won)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.PokerStats{}, nil
	}
	if err != nil {
		return user.PokerStats{}, errs.Wrap("r.conn.QueryRow", err)
	}

	return stats, nil
}

const countPokerQuery = `
	UPDATE poker
	SET hands = hands + $1,
		spent = spent + $2,
		won = won + $3
`

func (r *PokerRepository) Count(ctx context.Context, hands, spent, won int64) error {
	if _, err := r.conn(ctx).Exec(ctx, countPokerQuery, hands, spent, won); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const addPokerHandQuery = `
	INSERT INTO poker_hands (
		id,
		user_id,
		ante,
		cards,
		state,
		payout,
		started_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
`

func (r *PokerRepository) AddHand(ctx context.Context, h *user.PokerHand) error {
	cards := h.Cards()
	stored := make([]int16, len(cards))
	for i, c := range cards {
		stored[i] = int16(c)
	}

	_, err := r.conn(ctx).Exec(ctx, addPokerHandQuery, h.ID(), h.UserID(), h.Ante(), stored, string(h.State()), h.Payout(), h.StartedAt())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
		return user.ErrPokerOpen
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const setPokerHandOverQuery = `
	UPDATE poker_hands
	SET state = $2,
		payout = $3
	WHERE id = $1
`

func (r *PokerRepository) SetOver(ctx context.Context, h *user.PokerHand) error {
	tag, err := r.conn(ctx).Exec(ctx, setPokerHandOverQuery, h.ID(), string(h.State()), h.Payout())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrPokerNotFound
	}

	return nil
}

const getPokerHandQuery = `
	SELECT
		id,
		user_id,
		ante,
		cards,
		state,
		payout,
		started_at
	FROM poker_hands
`

func (r *PokerRepository) Hand(ctx context.Context, id uuid.UUID) (*user.PokerHand, error) {
	h, err := r.get(ctx, getPokerHandQuery+`WHERE id = $1`, id)

	return h, errs.Wrap("r.get", err)
}

func (r *PokerRepository) LockHand(ctx context.Context, id uuid.UUID) (*user.PokerHand, error) {
	h, err := r.get(ctx, getPokerHandQuery+`WHERE id = $1 FOR UPDATE`, id)

	return h, errs.Wrap("r.get", err)
}

func (r *PokerRepository) Dealt(ctx context.Context, userID uuid.UUID) (*user.PokerHand, error) {
	h, err := r.get(ctx, getPokerHandQuery+`WHERE user_id = $1 AND state = 'dealt'`, userID)

	return h, errs.Wrap("r.get", err)
}

func (r *PokerRepository) get(ctx context.Context, query string, arg uuid.UUID) (*user.PokerHand, error) {
	var (
		id, userID   uuid.UUID
		ante, payout int64
		stored       []int16
		state        string
		startedAt    time.Time
		cards        [9]user.Card
	)

	err := r.conn(ctx).QueryRow(ctx, query, arg).Scan(&id, &userID, &ante, &stored, &state, &payout, &startedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrPokerNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}

	for i := range min(len(stored), len(cards)) {
		cards[i] = user.Card(stored[i])
	}

	return user.RestorePokerHand(id, userID, ante, cards, user.PokerState(state), payout, startedAt), nil
}

const deletePokerHandsOverQuery = `
	DELETE FROM poker_hands
	WHERE user_id = $1
		AND state <> 'dealt'
		AND started_at < $2
`

func (r *PokerRepository) DeleteOver(ctx context.Context, userID uuid.UUID, before time.Time) error {
	if _, err := r.conn(ctx).Exec(ctx, deletePokerHandsOverQuery, userID, before); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
