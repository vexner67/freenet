package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// reactionQueries are those of one table of reactions: the table can not
// be a parameter, so each has its own.
type reactionQueries struct {
	set, remove, of, forget string
}

// The reaction is put only on a message that is there: none is a
// message gone.
var dialogReactionQueries = reactionQueries{
	set: `
		INSERT INTO message_reactions (
			message_id,
			user_id,
			reaction,
			created_at
		)
		SELECT id, $2, $3, $4
		FROM messages
		WHERE id = $1
		ON CONFLICT (message_id, user_id) DO UPDATE
		SET reaction = EXCLUDED.reaction
	`,
	remove: `
		DELETE FROM message_reactions
		WHERE message_id = $1
			AND user_id = $2
	`,
	of: `
		SELECT
			message_id,
			user_id,
			reaction
		FROM message_reactions
		WHERE message_id = ANY($1)
		ORDER BY created_at, user_id
	`,
	forget: `
		DELETE FROM message_reactions
		WHERE user_id = $1
	`,
}

var chatReactionQueries = reactionQueries{
	set: `
		INSERT INTO chat_message_reactions (
			message_id,
			user_id,
			reaction,
			created_at
		)
		SELECT id, $2, $3, $4
		FROM chat_messages
		WHERE id = $1
		ON CONFLICT (message_id, user_id) DO UPDATE
		SET reaction = EXCLUDED.reaction
	`,
	remove: `
		DELETE FROM chat_message_reactions
		WHERE message_id = $1
			AND user_id = $2
	`,
	of: `
		SELECT
			message_id,
			user_id,
			reaction
		FROM chat_message_reactions
		WHERE message_id = ANY($1)
		ORDER BY created_at, user_id
	`,
	forget: `
		DELETE FROM chat_message_reactions
		WHERE user_id = $1
	`,
}

// ReactionRepository keeps the reactions to the messages of dialogs or of
// chats.
type ReactionRepository struct {
	pool    *pgxpool.Pool
	queries reactionQueries
}

// NewReactionRepository keeps those to the messages of dialogs.
func NewReactionRepository(pool *pgxpool.Pool) *ReactionRepository {
	return &ReactionRepository{pool: pool, queries: dialogReactionQueries}
}

// NewChatReactionRepository keeps those to the messages of chats.
func NewChatReactionRepository(pool *pgxpool.Pool) *ReactionRepository {
	return &ReactionRepository{pool: pool, queries: chatReactionQueries}
}

func (r *ReactionRepository) Set(ctx context.Context, messageID, userID uuid.UUID, reaction message.Reaction, now time.Time) error {
	tag, err := r.pool.Exec(ctx, r.queries.set, messageID, userID, string(reaction), now)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return message.ErrNotFound
	}

	return nil
}

func (r *ReactionRepository) Remove(ctx context.Context, messageID, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, r.queries.remove, messageID, userID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

func (r *ReactionRepository) Of(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]message.Reactions, error) {
	if len(messageIDs) == 0 {
		return map[uuid.UUID]message.Reactions{}, nil
	}

	rows, err := r.pool.Query(ctx, r.queries.of, messageIDs)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]message.Reactions)
	for rows.Next() {
		var (
			messageID, userID uuid.UUID
			reaction          string
		)
		if err := rows.Scan(&messageID, &userID, &reaction); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		out[messageID] = append(out[messageID], message.Reacted{UserID: userID, Reaction: message.Reaction(reaction)})
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return out, nil
}

func (r *ReactionRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, r.queries.forget, userID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
