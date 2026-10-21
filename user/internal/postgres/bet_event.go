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

// BetEventRepository keeps the events people stake on: bet_events and
// bet_stakes.
type BetEventRepository struct {
	pool *pgxpool.Pool
}

func NewBetEventRepository(pool *pgxpool.Pool) *BetEventRepository {
	return &BetEventRepository{
		pool: pool,
	}
}

func (r *BetEventRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const createBetEventQuery = `
	INSERT INTO bet_events (
		id,
		question,
		outcomes,
		odds,
		state,
		closes_at
	)
	VALUES ($1, $2, $3, $4, $5, $6)
`

func (r *BetEventRepository) Create(ctx context.Context, e *user.BetEvent) error {
	_, err := r.conn(ctx).Exec(ctx, createBetEventQuery,
		e.ID(),
		e.Question().String(),
		outcomeNames(e.Outcomes()),
		oddsOf(e),
		string(e.State()),
		nullTime(e.ClosesAt()),
	)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const updateBetEventQuery = `
	UPDATE bet_events
	SET state = $2,
		winner = $3,
		odds = $4
	WHERE id = $1
`

func (r *BetEventRepository) Update(ctx context.Context, e *user.BetEvent) error {
	var winner *int
	if w := e.Winner(); w >= 0 {
		winner = &w
	}

	tag, err := r.conn(ctx).Exec(ctx, updateBetEventQuery, e.ID(), string(e.State()), winner, oddsOf(e))
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrBetEventNotFound
	}

	return nil
}

const lockBetEventQuery = `
	SELECT
		id,
		question,
		outcomes,
		odds,
		state,
		closes_at,
		winner
	FROM bet_events
	WHERE id = $1
	FOR UPDATE
`

func (r *BetEventRepository) Lock(ctx context.Context, id uuid.UUID) (*user.BetEvent, error) {
	e, err := scanBetEvent(r.conn(ctx).QueryRow(ctx, lockBetEventQuery, id))

	return e, errs.Wrap("scanBetEvent", err)
}

// Those taking stakes or waiting to be resolved come first, then the
// newest; popular are only those taking stakes now, the most staked on
// first. The stakes are summed per outcome, those of the viewer apart.
const listBetEventsQuery = `
	SELECT
		e.id,
		e.question,
		e.outcomes,
		e.odds,
		e.state,
		e.closes_at,
		e.winner,
		coalesce(s.staked, '{}'),
		coalesce(s.bettors, '{}'),
		coalesce(s.outcomes, '{}'),
		coalesce(s.mine, '{}'),
		coalesce(s.potential, '{}'),
		coalesce(s.payout, 0),
		coalesce(p.people, 0)
	FROM bet_events e
	LEFT JOIN LATERAL (
		SELECT
			array_agg(o.staked) AS staked,
			array_agg(o.bettors) AS bettors,
			array_agg(o.outcome) AS outcomes,
			array_agg(o.mine) AS mine,
			array_agg(o.potential) AS potential,
			sum(o.payout)::bigint AS payout,
			sum(o.staked) AS total
		FROM (
			SELECT
				outcome,
				sum(amount)::bigint AS staked,
				count(*)::integer AS bettors,
				coalesce(sum(amount) FILTER (WHERE user_id = $2), 0)::bigint AS mine,
				coalesce(sum(potential) FILTER (WHERE user_id = $2), 0)::bigint AS potential,
				coalesce(sum(payout) FILTER (WHERE user_id = $2), 0)::bigint AS payout
			FROM bet_stakes
			WHERE event_id = e.id
			GROUP BY outcome
		) o
	) s ON true
	LEFT JOIN LATERAL (
		SELECT count(DISTINCT user_id)::integer AS people
		FROM bet_stakes
		WHERE event_id = e.id
	) p ON true
	WHERE NOT $1 OR (e.state = 'open' AND (e.closes_at IS NULL OR e.closes_at > now()))
	ORDER BY
		CASE WHEN $1 THEN coalesce(s.total, 0) END DESC,
		CASE WHEN $1 THEN coalesce(p.people, 0) END DESC,
		e.state IN ('resolved', 'cancelled'),
		e.id DESC
	LIMIT $3
`

func (r *BetEventRepository) List(ctx context.Context, list service.BetEventList, viewerID uuid.UUID) ([]service.BetEventView, error) {
	rows, err := r.conn(ctx).Query(ctx, listBetEventsQuery,
		list == service.BetEventsPopular,
		viewerID,
		service.MaxListedBetEvents,
	)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var views []service.BetEventView
	for rows.Next() {
		var (
			e                   betEventRow
			staked, mine        []int64
			potential           []int64
			bettors, outcomesOf []int32
			v                   service.BetEventView
		)

		if err = rows.Scan(
			&e.id, &e.question, &e.outcomes, &e.odds, &e.state, &e.closesAt, &e.winner,
			&staked, &bettors, &outcomesOf, &mine, &potential, &v.Payout, &v.People,
		); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		if v.Event, err = e.restore(); err != nil {
			return nil, errs.Wrap("e.restore", err)
		}

		n := len(e.outcomes)
		v.Staked, v.Bettors, v.Mine, v.Potential = make([]int64, n), make([]int, n), make([]int64, n), make([]int64, n)
		for i, o := range outcomesOf {
			if int(o) >= n {
				continue
			}
			v.Staked[o], v.Bettors[o], v.Mine[o], v.Potential[o] = staked[i], int(bettors[i]), mine[i], potential[i]
		}

		views = append(views, v)
	}

	return views, errs.Wrap("rows.Err", rows.Err())
}

const addBetStakeQuery = `
	INSERT INTO bet_stakes (
		event_id,
		user_id,
		outcome,
		amount,
		potential
	)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (event_id, user_id, outcome) DO UPDATE
	SET amount = bet_stakes.amount + EXCLUDED.amount,
		potential = bet_stakes.potential + EXCLUDED.potential
`

func (r *BetEventRepository) AddStake(ctx context.Context, eventID, userID uuid.UUID, outcome int, amount, potential int64) error {
	if _, err := r.conn(ctx).Exec(ctx, addBetStakeQuery, eventID, userID, outcome, amount, potential); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const listBetStakesQuery = `
	SELECT
		user_id,
		outcome,
		amount,
		potential,
		payout
	FROM bet_stakes
	WHERE event_id = $1
	ORDER BY user_id, outcome
`

func (r *BetEventRepository) Stakes(ctx context.Context, eventID uuid.UUID) ([]user.Stake, error) {
	rows, err := r.conn(ctx).Query(ctx, listBetStakesQuery, eventID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var stakes []user.Stake
	for rows.Next() {
		var s user.Stake
		if err = rows.Scan(&s.UserID, &s.Outcome, &s.Amount, &s.Potential, &s.Payout); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		stakes = append(stakes, s)
	}

	return stakes, errs.Wrap("rows.Err", rows.Err())
}

const setBetPayoutsQuery = `
	UPDATE bet_stakes s
	SET payout = p.payout
	FROM unnest($2::uuid[], $3::integer[], $4::bigint[]) AS p (user_id, outcome, payout)
	WHERE s.event_id = $1
		AND s.user_id = p.user_id
		AND s.outcome = p.outcome
`

func (r *BetEventRepository) SetPayouts(ctx context.Context, eventID uuid.UUID, stakes []user.Stake) error {
	users := make([]uuid.UUID, 0, len(stakes))
	outcomes := make([]int32, 0, len(stakes))
	payouts := make([]int64, 0, len(stakes))
	for _, s := range stakes {
		users = append(users, s.UserID)
		outcomes = append(outcomes, int32(s.Outcome)) //nolint:gosec // below user.MaxBetOutcomes
		payouts = append(payouts, s.Payout)
	}

	if _, err := r.conn(ctx).Exec(ctx, setBetPayoutsQuery, eventID, users, outcomes, payouts); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const deleteBetEventQuery = `
	DELETE FROM bet_events
	WHERE id = $1
`

func (r *BetEventRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteBetEventQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrBetEventNotFound
	}

	return nil
}

// betEventRow is a row of bet_events.
type betEventRow struct {
	id       uuid.UUID
	question string
	outcomes []string
	// odds are NULL for an event sharing its stakes.
	odds     []int32
	state    string
	closesAt *time.Time
	winner   *int
}

func scanBetEvent(row pgx.Row) (*user.BetEvent, error) {
	var e betEventRow

	err := row.Scan(&e.id, &e.question, &e.outcomes, &e.odds, &e.state, &e.closesAt, &e.winner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrBetEventNotFound
	}
	if err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}

	event, err := e.restore()

	return event, errs.Wrap("e.restore", err)
}

func (e betEventRow) restore() (*user.BetEvent, error) {
	question, err := user.NewBetQuestion(e.question)
	if err != nil {
		return nil, errs.Wrap("user.NewBetQuestion", err)
	}

	outcomes := make([]user.BetOutcome, 0, len(e.outcomes))
	for _, name := range e.outcomes {
		o, err := user.NewBetOutcome(name)
		if err != nil {
			return nil, errs.Wrap("user.NewBetOutcome", err)
		}
		outcomes = append(outcomes, o)
	}

	state, err := user.NewBetEventState(e.state)
	if err != nil {
		return nil, errs.Wrap("user.NewBetEventState", err)
	}

	var closesAt time.Time
	if e.closesAt != nil {
		closesAt = e.closesAt.UTC()
	}

	winner := -1
	if e.winner != nil {
		winner = *e.winner
	}

	var odds []int
	if e.odds != nil {
		odds = make([]int, len(e.odds))
		for i, o := range e.odds {
			odds[i] = int(o)
		}
	}

	return user.RestoreBetEvent(e.id, question, outcomes, odds, state, closesAt, winner), nil
}

// oddsOf are the odds of the event as the column keeps them: NULL for one
// sharing its stakes.
func oddsOf(e *user.BetEvent) []int32 {
	if e.Pooled() {
		return nil
	}

	odds := make([]int32, len(e.Odds()))
	for i, o := range e.Odds() {
		odds[i] = int32(o) //nolint:gosec // below user.MaxEventOdds
	}

	return odds
}

func outcomeNames(outcomes []user.BetOutcome) []string {
	names := make([]string, 0, len(outcomes))
	for _, o := range outcomes {
		names = append(names, o.String())
	}

	return names
}

// nullTime is NULL for the zero time.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}
