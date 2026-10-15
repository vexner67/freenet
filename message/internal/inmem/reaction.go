package inmem

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// Reactions are those to the messages of dialogs.
func (s *Store) Reactions() *ReactionRepository {
	return &ReactionRepository{s: s, of: func() map[uuid.UUID]message.Reactions { return s.reactions }, exists: func(id uuid.UUID) bool {
		_, ok := s.messages[id]
		return ok
	}}
}

// ChatReactions are those to the messages of chats.
func (s *Store) ChatReactions() *ReactionRepository {
	return &ReactionRepository{s: s, of: func() map[uuid.UUID]message.Reactions { return s.chatReactions }, exists: func(id uuid.UUID) bool {
		_, ok := s.chatMessages[id]
		return ok
	}}
}

// ReactionRepository keeps reactions by message, the oldest first, as
// the table does: a message that is gone takes none.
type ReactionRepository struct {
	s      *Store
	of     func() map[uuid.UUID]message.Reactions
	exists func(uuid.UUID) bool
}

func (r *ReactionRepository) Set(ctx context.Context, messageID, userID uuid.UUID, reaction message.Reaction, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if !r.exists(messageID) {
		return message.ErrNotFound
	}

	all := r.of()
	reactions := all[messageID]

	if i := slices.IndexFunc(reactions, func(x message.Reacted) bool { return x.UserID == userID }); i >= 0 {
		reactions = slices.Clone(reactions)
		reactions[i].Reaction = reaction
		all[messageID] = reactions

		return nil
	}

	all[messageID] = append(slices.Clone(reactions), message.Reacted{UserID: userID, Reaction: reaction})

	return nil
}

func (r *ReactionRepository) Remove(ctx context.Context, messageID, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	all := r.of()
	all[messageID] = slices.DeleteFunc(slices.Clone(all[messageID]), func(x message.Reacted) bool { return x.UserID == userID })

	return nil
}

func (r *ReactionRepository) Of(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]message.Reactions, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	out := make(map[uuid.UUID]message.Reactions)
	for _, id := range messageIDs {
		if reactions := r.of()[id]; len(reactions) > 0 && r.exists(id) {
			out[id] = reactions
		}
	}

	return out, nil
}

func (r *ReactionRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	all := r.of()
	for id, reactions := range all {
		all[id] = slices.DeleteFunc(slices.Clone(reactions), func(x message.Reacted) bool { return x.UserID == userID })
	}

	return nil
}
