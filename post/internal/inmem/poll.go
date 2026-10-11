package inmem

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// pollVote is what a user chose in a poll, and when.
type pollVote struct {
	choice post.PollChoice
	at     time.Time
}

// PollRepository keeps polls in the Store of their posts: a poll goes
// with its post, as the foreign key does in PostgreSQL.
type PollRepository struct {
	s *Store
}

func (s *Store) Polls() *PollRepository {
	return &PollRepository{s: s}
}

func (r *PollRepository) Save(ctx context.Context, p *post.Poll) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.posts[p.PostID()]; !ok {
		return post.ErrNotFound
	}

	r.s.polls[p.PostID()] = p

	return nil
}

func (r *PollRepository) Get(ctx context.Context, postID uuid.UUID) (*post.Poll, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	p, ok := r.s.polls[postID]
	if !ok {
		return nil, post.ErrPollNotFound
	}

	return p, nil
}

// Lock is Get: the in-memory store has no transactions to hold a poll in.
func (r *PollRepository) Lock(ctx context.Context, postID uuid.UUID) (*post.Poll, error) {
	return r.Get(ctx, postID)
}

func (r *PollRepository) GetMany(ctx context.Context, postIDs []uuid.UUID) ([]*post.Poll, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var polls []*post.Poll
	for _, id := range postIDs {
		if p, ok := r.s.polls[id]; ok {
			polls = append(polls, p)
		}
	}

	return polls, nil
}

func (r *PollRepository) Chosen(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]post.PollChoice, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	chosen := make(map[uuid.UUID]post.PollChoice)
	for _, id := range postIDs {
		if v, ok := r.s.pollVotes[id][userID]; ok {
			chosen[id] = v.choice
		}
	}

	return chosen, nil
}

func (r *PollRepository) Vote(ctx context.Context, postID, userID uuid.UUID, choice post.PollChoice, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	p, ok := r.s.polls[postID]
	if !ok {
		return post.ErrPollNotFound
	}

	votes := r.s.pollVotes[postID]
	if votes == nil {
		votes = make(map[uuid.UUID]pollVote)
		r.s.pollVotes[postID] = votes
	}

	if _, ok = votes[userID]; ok {
		return post.ErrPollVoted
	}

	votes[userID] = pollVote{choice: choice, at: at.UTC().Truncate(time.Microsecond)} // as PostgreSQL keeps it
	r.s.polls[postID] = p.Counted(choice, 1)

	return nil
}

func (r *PollRepository) Retract(ctx context.Context, postID, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	r.s.retractVote(postID, userID)

	return nil
}

// retractVote takes a user's vote in a poll back and uncounts it.
func (s *Store) retractVote(postID, userID uuid.UUID) {
	v, ok := s.pollVotes[postID][userID]
	if !ok {
		return
	}

	delete(s.pollVotes[postID], userID)
	s.polls[postID] = s.polls[postID].Counted(v.choice, -1)
}

func (r *PollRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for postID := range r.s.pollVotes {
		r.s.retractVote(postID, userID)
	}

	return nil
}

func (r *PollRepository) Voters(ctx context.Context, postID uuid.UUID, place int, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	users := make(map[uuid.UUID]time.Time)
	for userID, v := range r.s.pollVotes[postID] {
		for _, chosen := range v.choice {
			if chosen == place {
				users[userID] = v.at
			}
		}
	}

	return likers(users, after, limit), nil
}
