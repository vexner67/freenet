package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// HoldemRepository keeps the tables of "Покер" in poker_tables.
type HoldemRepository struct {
	pool *pgxpool.Pool
}

func NewHoldemRepository(pool *pgxpool.Pool) *HoldemRepository {
	return &HoldemRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *HoldemRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// storedHoldem is a table as the state column keeps it.
type storedHoldem struct {
	Name       string           `json:"name"`
	OwnerID    uuid.UUID        `json:"ownerId"`
	Private    bool             `json:"private"`
	BigBlind   int64            `json:"bigBlind"`
	Players    []storedSeat     `json:"players"`
	Invited    []uuid.UUID      `json:"invited"`
	Street     user.TableStreet `json:"street"`
	Hand       int64            `json:"hand"`
	Button     int              `json:"button"`
	Turn       int              `json:"turn"`
	Deadline   time.Time        `json:"deadline"`
	Deck       []user.Card      `json:"deck"`
	Board      []user.Card      `json:"board"`
	Pot        int64            `json:"pot"`
	CurrentBet int64            `json:"currentBet"`
	MinRaise   int64            `json:"minRaise"`
	Version    int64            `json:"version"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type storedSeat struct {
	UserID    uuid.UUID        `json:"userId"`
	Stack     int64            `json:"stack"`
	Cards     []user.Card      `json:"cards"`
	Bet       int64            `json:"bet"`
	Put       int64            `json:"put"`
	InHand    bool             `json:"inHand"`
	Folded    bool             `json:"folded"`
	AllIn     bool             `json:"allIn"`
	Acted     bool             `json:"acted"`
	Away      bool             `json:"away"`
	AwaySince time.Time        `json:"awaySince"`
	Missed    int              `json:"missed"`
	Leaving   bool             `json:"leaving"`
	Shown     bool             `json:"shown"`
	Won       int64            `json:"won"`
	Last      user.TableAction `json:"last"`
}

func storeHoldem(t *user.HoldemTable) ([]byte, error) {
	s := t.State()

	stored := storedHoldem{
		Name: s.Name, OwnerID: s.OwnerID, Private: s.Private, BigBlind: s.BigBlind, Invited: s.Invited,
		Street: s.Street, Hand: s.Hand, Button: s.Button, Turn: s.Turn, Deadline: s.Deadline, Deck: s.Deck,
		Board: s.Board, Pot: s.Pot, CurrentBet: s.CurrentBet, MinRaise: s.MinRaise, Version: s.Version,
		CreatedAt: s.CreatedAt,
		Players:   make([]storedSeat, len(s.Players)),
	}
	for i, p := range s.Players {
		stored.Players[i] = storedSeat(p)
	}

	out, err := json.Marshal(stored)

	return out, errs.Wrap("json.Marshal", err)
}

func restoreHoldem(id uuid.UUID, raw []byte) (*user.HoldemTable, error) {
	var stored storedHoldem
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, errs.Wrap("json.Unmarshal", err)
	}

	s := user.HoldemState{
		Name: stored.Name, OwnerID: stored.OwnerID, Private: stored.Private, BigBlind: stored.BigBlind,
		Invited: stored.Invited, Street: stored.Street, Hand: stored.Hand, Button: stored.Button,
		Turn: stored.Turn, Deadline: stored.Deadline, Deck: stored.Deck, Board: stored.Board, Pot: stored.Pot,
		CurrentBet: stored.CurrentBet, MinRaise: stored.MinRaise, Version: stored.Version,
		CreatedAt: stored.CreatedAt,
		Players:   make([]user.TableSeat, len(stored.Players)),
	}
	for i, p := range stored.Players {
		s.Players[i] = user.TableSeat(p)
	}

	return user.RestoreHoldemTable(id, s), nil
}

// columns are the columns beside state that the queries look for.
func columns(t *user.HoldemTable) (players int, playerIDs []uuid.UUID, due *time.Time) {
	for _, p := range t.Seats() {
		if p.Taken() {
			players++
			playerIDs = append(playerIDs, p.UserID)
		}
	}
	if d := t.Due(); !d.IsZero() {
		due = &d
	}

	return players, playerIDs, due
}

const addHoldemTableQuery = `
	INSERT INTO poker_tables (
		id,
		private,
		big_blind,
		seats,
		players,
		player_ids,
		invited_ids,
		due_at,
		state,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`

func (r *HoldemRepository) AddTable(ctx context.Context, t *user.HoldemTable) error {
	state, err := storeHoldem(t)
	if err != nil {
		return errs.Wrap("storeHoldem", err)
	}

	players, playerIDs, due := columns(t)

	_, err = r.conn(ctx).Exec(ctx, addHoldemTableQuery, t.ID(), t.Private(), t.BigBlind(), len(t.Seats()), players,
		uuids(playerIDs), uuids(t.Invited()), due, state, t.CreatedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// uuids is never nil: the columns are NOT NULL.
func uuids(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}

	return ids
}

const getHoldemTableQuery = `
	SELECT
		id,
		state
	FROM poker_tables
`

func (r *HoldemRepository) Table(ctx context.Context, id uuid.UUID) (*user.HoldemTable, error) {
	t, err := r.get(ctx, getHoldemTableQuery+`WHERE id = $1`, id)

	return t, errs.Wrap("r.get", err)
}

func (r *HoldemRepository) LockTable(ctx context.Context, id uuid.UUID) (*user.HoldemTable, error) {
	t, err := r.get(ctx, getHoldemTableQuery+`WHERE id = $1 FOR UPDATE`, id)

	return t, errs.Wrap("r.get", err)
}

const lockFreeHoldemTableQuery = getHoldemTableQuery + `
	WHERE NOT private
		AND big_blind = $1
		AND players < seats
		AND NOT ($2 = ANY (player_ids))
	ORDER BY players DESC, id
	LIMIT 1
	FOR UPDATE SKIP LOCKED
`

func (r *HoldemRepository) LockFree(ctx context.Context, bigBlind int64, userID uuid.UUID) (*user.HoldemTable, error) {
	t, err := r.get(ctx, lockFreeHoldemTableQuery, bigBlind, userID)

	return t, errs.Wrap("r.get", err)
}

func (r *HoldemRepository) get(ctx context.Context, query string, args ...any) (*user.HoldemTable, error) {
	var (
		id  uuid.UUID
		raw []byte
	)

	err := r.conn(ctx).QueryRow(ctx, query, args...).Scan(&id, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrTableNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}

	t, err := restoreHoldem(id, raw)

	return t, errs.Wrap("restoreHoldem", err)
}

const saveHoldemTableQuery = `
	UPDATE poker_tables
	SET players = $2,
		player_ids = $3,
		invited_ids = $4,
		due_at = $5,
		state = $6
	WHERE id = $1
`

func (r *HoldemRepository) SaveTable(ctx context.Context, t *user.HoldemTable) error {
	state, err := storeHoldem(t)
	if err != nil {
		return errs.Wrap("storeHoldem", err)
	}

	players, playerIDs, due := columns(t)

	tag, err := r.conn(ctx).Exec(ctx, saveHoldemTableQuery, t.ID(), players, uuids(playerIDs), uuids(t.Invited()), due, state)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrTableNotFound
	}

	return nil
}

const deleteHoldemTableQuery = `
	DELETE FROM poker_tables
	WHERE id = $1
`

func (r *HoldemRepository) DeleteTable(ctx context.Context, id uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteHoldemTableQuery, id); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

var holdemSections = map[service.TableSection]string{
	service.TablesPublic:  `WHERE NOT private AND $1::uuid IS NOT NULL ORDER BY players DESC, id DESC`,
	service.TablesMine:    `WHERE $1 = ANY (player_ids) ORDER BY id DESC`,
	service.TablesInvited: `WHERE $1 = ANY (invited_ids) AND NOT ($1 = ANY (player_ids)) ORDER BY id DESC`,
}

func (r *HoldemRepository) Tables(ctx context.Context, section service.TableSection, userID uuid.UUID) ([]*user.HoldemTable, error) {
	where, ok := holdemSections[section]
	if !ok {
		return nil, service.ErrTableSection
	}

	query := fmt.Sprintf("%s%s LIMIT %d", getHoldemTableQuery, where, service.MaxListedTables)

	rows, err := r.conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var tables []*user.HoldemTable
	for rows.Next() {
		var (
			id  uuid.UUID
			raw []byte
		)
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		t, err := restoreHoldem(id, raw)
		if err != nil {
			return nil, errs.Wrap("restoreHoldem", err)
		}
		tables = append(tables, t)
	}

	return tables, errs.Wrap("rows.Err", rows.Err())
}

const dueHoldemTablesQuery = `
	SELECT id
	FROM poker_tables
	WHERE due_at <= $1
	ORDER BY due_at
	LIMIT $2
`

func (r *HoldemRepository) Due(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, dueHoldemTablesQuery, now, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])

	return ids, errs.Wrap("pgx.CollectRows", err)
}
