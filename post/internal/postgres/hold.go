package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// HoldRepository keeps the likes and the poll votes of deleted pages in
// held_reactions, out of the tables every count and list reads.
type HoldRepository struct {
	pool *pgxpool.Pool
}

func NewHoldRepository(pool *pgxpool.Pool) *HoldRepository {
	return &HoldRepository{
		pool: pool,
	}
}

func (r *HoldRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// likeTable is a table of likes: one row per user and target, counted in
// like_count of the target.
type likeTable struct {
	kind    string
	likes   string
	column  string
	targets string
}

// The five tables of likes differ in names only, so their statements are
// built from these, which are constants of this file, never input.
var likeTables = []likeTable{
	{kind: "post_like", likes: "post_likes", column: "post_id", targets: "posts"},
	{kind: "comment_like", likes: "comment_likes", column: "comment_id", targets: "comments"},
	{kind: "album_photo_like", likes: "album_photo_likes", column: "photo_id", targets: "album_photos"},
	{kind: "topic_message_like", likes: "topic_message_likes", column: "message_id", targets: "topic_messages"},
	{kind: "photo_comment_like", likes: "photo_comment_likes", column: "comment_id", targets: "photo_comments"},
}

// holdLikesQuery moves a user's likes aside and uncounts them in one
// statement.
const holdLikesQuery = `
	WITH taken AS (
		DELETE FROM %[2]s
		WHERE user_id = $1
		RETURNING %[3]s AS target_id, created_at
	), held AS (
		INSERT INTO held_reactions (
			user_id,
			kind,
			target_id,
			created_at
		)
		SELECT $1, '%[1]s', target_id, created_at
		FROM taken
		ON CONFLICT DO NOTHING
	)
	UPDATE %[4]s t
	SET like_count = t.like_count - 1
	FROM taken
	WHERE t.id = taken.target_id
`

// releaseLikesQuery puts the likes back and counts them in one statement:
// those whose target is still there, the rest are dropped.
const releaseLikesQuery = `
	WITH back AS (
		DELETE FROM held_reactions
		WHERE user_id = $1
			AND kind = '%[1]s'
		RETURNING target_id, created_at
	), put AS (
		INSERT INTO %[2]s (
			%[3]s,
			user_id,
			created_at
		)
		SELECT back.target_id, $1, back.created_at
		FROM back
		JOIN %[4]s t ON t.id = back.target_id
		ON CONFLICT DO NOTHING
		RETURNING %[3]s AS target_id
	)
	UPDATE %[4]s t
	SET like_count = t.like_count + 1
	FROM put
	WHERE t.id = put.target_id
`

func (t likeTable) query(format string) string {
	return fmt.Sprintf(format, t.kind, t.likes, t.column, t.targets)
}

// holdVotesQuery moves a user's votes aside and uncounts them, as
// deleteVotesByUserQuery does.
const holdVotesQuery = `
	WITH taken AS (
		DELETE FROM poll_votes
		WHERE user_id = $1
		RETURNING post_id, place, created_at
	), held AS (
		INSERT INTO held_reactions (
			user_id,
			kind,
			target_id,
			place,
			created_at
		)
		SELECT $1, 'poll_vote', post_id, place, created_at
		FROM taken
		ON CONFLICT DO NOTHING
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

// releaseVotesQuery puts the votes back in the polls that are still
// there and counts them.
const releaseVotesQuery = `
	WITH back AS (
		DELETE FROM held_reactions
		WHERE user_id = $1
			AND kind = 'poll_vote'
		RETURNING target_id, place, created_at
	), put AS (
		INSERT INTO poll_votes (
			post_id,
			place,
			user_id,
			created_at
		)
		SELECT back.target_id, back.place, $1, back.created_at
		FROM back
		JOIN poll_options o ON o.post_id = back.target_id
			AND o.place = back.place
		ON CONFLICT DO NOTHING
		RETURNING post_id, place
	), counted AS (
		UPDATE poll_options o
		SET vote_count = o.vote_count + 1
		FROM put
		WHERE o.post_id = put.post_id
			AND o.place = put.place
	)
	UPDATE polls
	SET voter_count = voter_count + 1
	WHERE post_id IN (SELECT post_id FROM put)
`

func (r *HoldRepository) Hold(ctx context.Context, userID uuid.UUID) error {
	return r.each(ctx, userID, holdLikesQuery, holdVotesQuery)
}

func (r *HoldRepository) Release(ctx context.Context, userID uuid.UUID) error {
	return r.each(ctx, userID, releaseLikesQuery, releaseVotesQuery)
}

// each runs the statement of every table of likes and that of the votes.
func (r *HoldRepository) each(ctx context.Context, userID uuid.UUID, likes, votes string) error {
	for _, t := range likeTables {
		if _, err := r.conn(ctx).Exec(ctx, t.query(likes), userID); err != nil {
			return errs.Wrap("r.conn.Exec", err)
		}
	}

	if _, err := r.conn(ctx).Exec(ctx, votes, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const forgetHeldQuery = `
	DELETE FROM held_reactions
	WHERE user_id = $1
`

func (r *HoldRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, forgetHeldQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
