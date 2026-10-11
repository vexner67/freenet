package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

const uniqueViolation = "23505"

type PollRepository struct {
	pool *pgxpool.Pool
}

func NewPollRepository(pool *pgxpool.Pool) *PollRepository {
	return &PollRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *PollRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// savePollQuery adds a poll with its options, in their order, in one
// statement.
const savePollQuery = `
	WITH poll AS (
		INSERT INTO polls (
			post_id,
			title,
			anonymous,
			multiple,
			locked,
			ends_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING post_id
	)
	INSERT INTO poll_options (
		post_id,
		place,
		name
	)
	SELECT poll.post_id, o.place - 1, o.name
	FROM poll, unnest($7::text[]) WITH ORDINALITY AS o (name, place)
`

func (r *PollRepository) Save(ctx context.Context, p *post.Poll) error {
	s := p.Stored()

	names := make([]string, len(s.Options))
	for i, o := range s.Options {
		names[i] = o.String()
	}

	_, err := r.conn(ctx).Exec(ctx, savePollQuery, s.PostID, s.Title.String(), s.Anonymous, s.Multiple, s.Locked, nullTime(s.EndsAt), names)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// nullTime is NULL for the zero time.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}

const getPollsQuery = `
	SELECT
		p.post_id,
		p.title,
		p.anonymous,
		p.multiple,
		p.locked,
		p.ends_at,
		p.voter_count,
		(SELECT array_agg(o.name ORDER BY o.place) FROM poll_options o WHERE o.post_id = p.post_id) AS names,
		(SELECT array_agg(o.vote_count ORDER BY o.place) FROM poll_options o WHERE o.post_id = p.post_id) AS votes
	FROM polls p
	WHERE p.post_id = ANY($1)
`

func (r *PollRepository) GetMany(ctx context.Context, postIDs []uuid.UUID) ([]*post.Poll, error) {
	rows, err := r.conn(ctx).Query(ctx, getPollsQuery, postIDs)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	polls, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.Poll, error) {
		return scanPoll(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return polls, nil
}

func (r *PollRepository) Get(ctx context.Context, postID uuid.UUID) (*post.Poll, error) {
	return r.one(ctx, getPollsQuery, postID)
}

// lockPollQuery is getPollsQuery of one poll that holds its row: the votes
// of a poll are cast and taken back one after another.
const lockPollQuery = `
	SELECT
		p.post_id,
		p.title,
		p.anonymous,
		p.multiple,
		p.locked,
		p.ends_at,
		p.voter_count,
		(SELECT array_agg(o.name ORDER BY o.place) FROM poll_options o WHERE o.post_id = p.post_id) AS names,
		(SELECT array_agg(o.vote_count ORDER BY o.place) FROM poll_options o WHERE o.post_id = p.post_id) AS votes
	FROM polls p
	WHERE p.post_id = ANY($1)
	FOR UPDATE OF p
`

func (r *PollRepository) Lock(ctx context.Context, postID uuid.UUID) (*post.Poll, error) {
	return r.one(ctx, lockPollQuery, postID)
}

func (r *PollRepository) one(ctx context.Context, query string, postID uuid.UUID) (*post.Poll, error) {
	p, err := scanPoll(r.conn(ctx).QueryRow(ctx, query, []uuid.UUID{postID}))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrPollNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanPoll", err)
	}

	return p, nil
}

func scanPoll(row pgx.Row) (*post.Poll, error) {
	var (
		s      post.StoredPoll
		title  string
		endsAt *time.Time
		names  []string
		votes  []int32
	)

	if err := row.Scan(&s.PostID, &title, &s.Anonymous, &s.Multiple, &s.Locked, &endsAt, &s.Tally.Voters, &names, &votes); err != nil {
		return nil, err
	}

	// Data comes from our own tables and was validated on write.
	s.Title = post.PollLine(title)
	if endsAt != nil {
		s.EndsAt = endsAt.UTC()
	}

	s.Options = make(post.PollOptions, len(names))
	s.Tally.Votes = make([]int, len(names))
	for i, name := range names {
		s.Options[i] = post.PollLine(name)
		s.Tally.Votes[i] = int(votes[i])
	}

	return post.RestorePoll(s), nil
}

const listChosenQuery = `
	SELECT
		post_id,
		array_agg(place ORDER BY place) AS places
	FROM poll_votes
	WHERE user_id = $1
		AND post_id = ANY($2)
	GROUP BY post_id
`

func (r *PollRepository) Chosen(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]post.PollChoice, error) {
	rows, err := r.conn(ctx).Query(ctx, listChosenQuery, userID, postIDs)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	var (
		postID uuid.UUID
		places []int16
	)

	chosen := make(map[uuid.UUID]post.PollChoice)

	_, err = pgx.ForEachRow(rows, []any{&postID, &places}, func() error {
		choice := make(post.PollChoice, len(places))
		for i, place := range places {
			choice[i] = int(place)
		}
		chosen[postID] = choice

		return nil
	})
	if err != nil {
		return nil, errs.Wrap("pgx.ForEachRow", err)
	}

	return chosen, nil
}

// votePollQuery adds the votes of a user and counts them, in the options
// and in the poll, in one statement.
const votePollQuery = `
	WITH cast_votes AS (
		INSERT INTO poll_votes (
			post_id,
			place,
			user_id,
			created_at
		)
		SELECT $1, place, $2, $4
		FROM unnest($3::smallint[]) AS place
		RETURNING place
	), counted AS (
		UPDATE poll_options o
		SET vote_count = o.vote_count + 1
		WHERE o.post_id = $1
			AND o.place IN (SELECT place FROM cast_votes)
	)
	UPDATE polls
	SET voter_count = voter_count + 1
	WHERE post_id = $1
`

func (r *PollRepository) Vote(ctx context.Context, postID, userID uuid.UUID, choice post.PollChoice, at time.Time) error {
	places := make([]int16, len(choice))
	for i, place := range choice {
		places[i] = int16(place) //nolint:gosec // a place is below post.MaxPollOptions
	}

	_, err := r.conn(ctx).Exec(ctx, votePollQuery, postID, userID, places, at.UTC().Truncate(time.Microsecond))

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return post.ErrPollVoted
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// retractVoteQuery takes the votes of a user back and uncounts them in
// one statement; nothing changes for one who has not voted.
const retractVoteQuery = `
	WITH taken AS (
		DELETE FROM poll_votes
		WHERE post_id = $1
			AND user_id = $2
		RETURNING place
	), uncounted AS (
		UPDATE poll_options o
		SET vote_count = o.vote_count - 1
		WHERE o.post_id = $1
			AND o.place IN (SELECT place FROM taken)
	)
	UPDATE polls
	SET voter_count = voter_count - 1
	WHERE post_id = $1
		AND EXISTS (SELECT 1 FROM taken)
`

func (r *PollRepository) Retract(ctx context.Context, postID, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, retractVoteQuery, postID, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// deleteVotesByUserQuery takes every vote of a user back and uncounts
// them in one statement: a user has one vote for an option, and is one
// voter of a poll.
const deleteVotesByUserQuery = `
	WITH taken AS (
		DELETE FROM poll_votes
		WHERE user_id = $1
		RETURNING post_id, place
	), uncounted AS (
		UPDATE poll_options o
		SET vote_count = o.vote_count - 1
		FROM taken t
		WHERE o.post_id = t.post_id
			AND o.place = t.place
	)
	UPDATE polls
	SET voter_count = voter_count - 1
	WHERE post_id IN (SELECT post_id FROM taken)
`

func (r *PollRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteVotesByUserQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const listPollVotersQuery = `
	SELECT
		user_id,
		created_at
	FROM poll_votes
	WHERE post_id = $1
		AND place = $2
		AND (created_at, user_id) < ($3, $4)
	ORDER BY created_at DESC, user_id DESC
	LIMIT $5
`

func (r *PollRepository) Voters(ctx context.Context, postID uuid.UUID, place int, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	at, userID := likersFrom(after)

	rows, err := r.conn(ctx).Query(ctx, listPollVotersQuery, postID, place, at, userID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectLikers(rows)
}
