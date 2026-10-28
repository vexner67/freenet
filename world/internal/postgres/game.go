package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/world/internal/world"
)

// GameRepository keeps the games of the worlds (world_games) as one
// document each.
type GameRepository struct {
	pool *pgxpool.Pool
}

func NewGameRepository(pool *pgxpool.Pool) *GameRepository {
	return &GameRepository{
		pool: pool,
	}
}

func (r *GameRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

type storedFlag struct {
	Pattern     string `json:"pattern"`
	Colors      []int  `json:"colors"`
	Emblem      string `json:"emblem,omitempty"`
	EmblemColor int    `json:"emblemColor,omitempty"`
	EmblemAt    string `json:"emblemAt,omitempty"`
}

type storedBudgetHour struct {
	Hour         time.Time `json:"hour"`
	Income       int64     `json:"income,omitempty"`
	Donations    int64     `json:"donations,omitempty"`
	Founding     int64     `json:"founding,omitempty"`
	Colonization int64     `json:"colonization,omitempty"`
}

type storedState struct {
	ID             uuid.UUID          `json:"id"`
	Name           string             `json:"name"`
	Adjective      string             `json:"adjective,omitempty"`
	Motto          string             `json:"motto,omitempty"`
	Color          int                `json:"color"`
	Flag           storedFlag         `json:"flag"`
	Government     string             `json:"government"`
	RulerID        uuid.UUID          `json:"rulerId"`
	Capital        int                `json:"capital"`
	Treasury       int64              `json:"treasury"`
	FoundedAt      time.Time          `json:"foundedAt"`
	ProtectedUntil time.Time          `json:"protectedUntil"`
	ClubID         uuid.UUID          `json:"clubId"`
	Budget         []storedBudgetHour `json:"budget,omitempty"`
}

type storedCitizen struct {
	UserID   uuid.UUID `json:"userId"`
	StateID  uuid.UUID `json:"stateId"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

type storedApplication struct {
	UserID  uuid.UUID `json:"userId"`
	StateID uuid.UUID `json:"stateId"`
	At      time.Time `json:"at"`
}

type storedColony struct {
	Province int       `json:"province"`
	StateID  uuid.UUID `json:"stateId"`
	Until    time.Time `json:"until"`
}

type storedGame struct {
	Hour         time.Time           `json:"hour"`
	States       []storedState       `json:"states"`
	Owners       map[string]string   `json:"owners"`
	Citizens     []storedCitizen     `json:"citizens"`
	Applications []storedApplication `json:"applications"`
	Colonies     []storedColony      `json:"colonies"`
}

func storeGame(g *world.Game) storedGame {
	s := g.Snapshot()
	out := storedGame{Hour: s.Hour, Owners: make(map[string]string, len(s.Owners))}
	for _, st := range s.States {
		f := st.Flag.Parts()
		ss := storedState{
			ID: st.ID, Name: st.Name.String(), Adjective: st.Adjective.String(), Motto: st.Motto.String(),
			Color: int(st.Color), Government: string(st.Government), RulerID: st.RulerID, Capital: st.Capital,
			Treasury: st.Treasury, FoundedAt: st.FoundedAt, ProtectedUntil: st.ProtectedUntil, ClubID: st.ClubID,
			Flag: storedFlag{
				Pattern: string(f.Pattern), Colors: f.Colors, Emblem: string(f.Emblem), EmblemColor: f.EmblemColor,
				EmblemAt: string(f.EmblemAt),
			},
		}
		for _, h := range st.Budget {
			ss.Budget = append(ss.Budget, storedBudgetHour(h))
		}
		out.States = append(out.States, ss)
	}
	for p, id := range s.Owners {
		out.Owners[strconv.Itoa(p)] = id.String()
	}
	for _, c := range s.Citizens {
		out.Citizens = append(out.Citizens, storedCitizen{UserID: c.UserID, StateID: c.StateID, Role: string(c.Role), JoinedAt: c.JoinedAt})
	}
	for _, a := range s.Applications {
		out.Applications = append(out.Applications, storedApplication(a))
	}
	for _, c := range s.Colonies {
		out.Colonies = append(out.Colonies, storedColony(c))
	}

	return out
}

func restoreGame(s storedGame) (*world.Game, error) {
	out := world.GameState{Hour: s.Hour, Owners: make(map[int]uuid.UUID, len(s.Owners))}
	for _, ss := range s.States {
		st := world.State{
			ID: ss.ID, Name: world.StateName(ss.Name), Adjective: world.Adjective(ss.Adjective), Motto: world.Motto(ss.Motto),
			Color: world.Color(ss.Color), Government: world.Government(ss.Government), RulerID: ss.RulerID, Capital: ss.Capital,
			Treasury: ss.Treasury, FoundedAt: ss.FoundedAt, ProtectedUntil: ss.ProtectedUntil, ClubID: ss.ClubID,
			Flag: world.RestoreFlag(world.FlagParts{
				Pattern: world.Pattern(ss.Flag.Pattern), Colors: ss.Flag.Colors, Emblem: world.Emblem(ss.Flag.Emblem),
				EmblemColor: ss.Flag.EmblemColor, EmblemAt: world.EmblemPlace(ss.Flag.EmblemAt),
			}),
		}
		for _, h := range ss.Budget {
			st.Budget = append(st.Budget, world.BudgetHour(h))
		}
		out.States = append(out.States, st)
	}
	for p, id := range s.Owners {
		province, err := strconv.Atoi(p)
		if err != nil {
			return nil, errs.Wrap("strconv.Atoi", err)
		}
		state, err := uuid.Parse(id)
		if err != nil {
			return nil, errs.Wrap("uuid.Parse", err)
		}
		out.Owners[province] = state
	}
	for _, c := range s.Citizens {
		out.Citizens = append(out.Citizens, world.Citizen{UserID: c.UserID, StateID: c.StateID, Role: world.Role(c.Role), JoinedAt: c.JoinedAt})
	}
	for _, a := range s.Applications {
		out.Applications = append(out.Applications, world.Application(a))
	}
	for _, c := range s.Colonies {
		out.Colonies = append(out.Colonies, world.Colony(c))
	}

	return world.RestoreGame(out), nil
}

const createGameQuery = `
	INSERT INTO world_games (
		world_id,
		game
	)
	VALUES ($1, $2)
	ON CONFLICT (world_id) DO NOTHING
`

func (r *GameRepository) CreateGame(ctx context.Context, worldID uuid.UUID, g *world.Game) error {
	data, err := json.Marshal(storeGame(g))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.conn(ctx).Exec(ctx, createGameQuery, worldID, data)

	return errs.Wrap("r.conn.Exec", err)
}

const (
	gameQuery       = `SELECT game FROM world_games WHERE world_id = $1`
	lockedGameQuery = gameQuery + ` FOR UPDATE`
)

func (r *GameRepository) Game(ctx context.Context, worldID uuid.UUID, lock bool) (*world.Game, error) {
	query := gameQuery
	if lock {
		query = lockedGameQuery
	}

	var s storedGame
	err := r.conn(ctx).QueryRow(ctx, query, worldID).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, world.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}

	g, err := restoreGame(s)

	return g, errs.Wrap("restoreGame", err)
}

const saveGameQuery = `
	UPDATE world_games
	SET game = $2,
		updated_at = now()
	WHERE world_id = $1
`

func (r *GameRepository) SaveGame(ctx context.Context, worldID uuid.UUID, g *world.Game) error {
	data, err := json.Marshal(storeGame(g))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	tag, err := r.conn(ctx).Exec(ctx, saveGameQuery, worldID, data)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return world.ErrNotFound
	}

	return nil
}
