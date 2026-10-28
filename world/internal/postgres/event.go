package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/world/internal/world"
)

// EventRepository keeps the news of the worlds (world_events).
type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{
		pool: pool,
	}
}

func (r *EventRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// nullID is uuid.Nil as NULL.
func nullID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

const addEventQuery = `
	INSERT INTO world_events (
		id,
		world_id,
		kind,
		state_id,
		user_id,
		province,
		name,
		role,
		at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`

func (r *EventRepository) AddEvents(ctx context.Context, worldID uuid.UUID, events []world.Event) error {
	for _, e := range events {
		_, err := r.conn(ctx).Exec(ctx, addEventQuery,
			e.ID, worldID, string(e.Kind), e.StateID, nullID(e.UserID), e.Province, e.Name, string(e.Role), e.At)
		if err != nil {
			return errs.Wrap("r.conn.Exec", err)
		}
	}

	return nil
}

// eventsQuery reads one page of the news; $2 = uuid.Nil reads all the
// states, $3 = uuid.Nil starts from the newest.
const eventsQuery = `
	SELECT
		id,
		kind,
		state_id,
		COALESCE(user_id, '00000000-0000-0000-0000-000000000000'),
		COALESCE(province, 0),
		name,
		role,
		at
	FROM world_events
	WHERE world_id = $1
		AND ($2 = '00000000-0000-0000-0000-000000000000'::uuid OR state_id = $2)
		AND ($3 = '00000000-0000-0000-0000-000000000000'::uuid OR id < $3)
	ORDER BY id DESC
	LIMIT $4
`

func (r *EventRepository) Events(ctx context.Context, worldID, stateID, before uuid.UUID, limit int) ([]world.Event, error) {
	rows, err := r.conn(ctx).Query(ctx, eventsQuery, worldID, stateID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (world.Event, error) {
		var (
			e          world.Event
			kind, role string
		)
		err := row.Scan(&e.ID, &kind, &e.StateID, &e.UserID, &e.Province, &e.Name, &role, &e.At)
		e.Kind, e.Role = world.EventKind(kind), world.Role(role)

		return e, err
	})

	return events, errs.Wrap("pgx.CollectRows", err)
}
