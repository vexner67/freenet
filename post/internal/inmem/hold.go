package inmem

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// userLikes are the likes of users: target → users → when.
type userLikes = map[uuid.UUID]map[uuid.UUID]time.Time

// heldLikes are the likes of deleted pages, kept out of userLikes:
// user → target → when.
type heldLikes map[uuid.UUID]map[uuid.UUID]time.Time

// hold moves the user's likes aside, telling uncount of every target.
func (h heldLikes) hold(likes userLikes, userID uuid.UUID, uncount func(target uuid.UUID)) {
	for target, users := range likes {
		at, ok := users[userID]
		if !ok {
			continue
		}

		if h[userID] == nil {
			h[userID] = make(map[uuid.UUID]time.Time)
		}

		h[userID][target] = at
		delete(users, userID)
		uncount(target)
	}
}

// release puts back the likes whose target is still there, telling count
// of each, and drops the rest.
func (h heldLikes) release(likes userLikes, userID uuid.UUID, exists func(target uuid.UUID) bool, count func(target uuid.UUID)) {
	for target, at := range h[userID] {
		if !exists(target) {
			continue
		}

		if likes[target] == nil {
			likes[target] = make(map[uuid.UUID]time.Time)
		}

		if _, ok := likes[target][userID]; !ok {
			likes[target][userID] = at
			count(target)
		}
	}

	delete(h, userID)
}

// counted by the length of the map: nothing to tell.
func uncounted(uuid.UUID) {}

// HoldRepository keeps aside what deleted pages liked among posts and
// their comments and voted for in polls.
type HoldRepository struct {
	s *Store
}

func (s *Store) Holds() *HoldRepository {
	return &HoldRepository{s: s}
}

func (r *HoldRepository) Hold(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s := r.s

	s.mu.Lock()
	defer s.mu.Unlock()

	s.hidden[userID] = true

	s.heldLikes.hold(s.likes, userID, func(id uuid.UUID) { s.count(id, func(c *post.Counters) { c.Likes-- }) })
	s.heldCommentLikes.hold(s.commentLikes, userID, func(id uuid.UUID) {
		s.restoreComment(id, func(sc *post.StoredComment) { sc.Likes-- })
	})

	for postID, votes := range s.pollVotes {
		v, ok := votes[userID]
		if !ok {
			continue
		}

		if s.heldVotes[userID] == nil {
			s.heldVotes[userID] = make(map[uuid.UUID]pollVote)
		}

		s.heldVotes[userID][postID] = v
		s.retractVote(postID, userID)
	}

	return nil
}

func (r *HoldRepository) Release(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s := r.s

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.hidden, userID)

	s.heldLikes.release(s.likes, userID,
		func(id uuid.UUID) bool { return s.posts[id] != nil },
		func(id uuid.UUID) { s.count(id, func(c *post.Counters) { c.Likes++ }) })
	s.heldCommentLikes.release(s.commentLikes, userID,
		func(id uuid.UUID) bool { return s.comments[id] != nil },
		func(id uuid.UUID) { s.restoreComment(id, func(sc *post.StoredComment) { sc.Likes++ }) })

	for postID, v := range s.heldVotes[userID] {
		p, ok := s.polls[postID]
		if !ok {
			continue
		}

		if s.pollVotes[postID] == nil {
			s.pollVotes[postID] = make(map[uuid.UUID]pollVote)
		}

		if _, voted := s.pollVotes[postID][userID]; !voted {
			s.pollVotes[postID][userID] = v
			s.polls[postID] = p.Counted(v.choice, 1)
		}
	}

	delete(s.heldVotes, userID)

	return nil
}

func (r *HoldRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	delete(r.s.heldLikes, userID)
	delete(r.s.heldCommentLikes, userID)
	delete(r.s.heldVotes, userID)
	delete(r.s.hidden, userID)

	return nil
}

// Hold keeps aside the hearts a deleted page put on messages of topics.
func (r *TopicRepository) Hold(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.hidden[userID] = true
	r.heldLikes.hold(r.likes, userID, uncounted)

	return nil
}

func (r *TopicRepository) Release(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.hidden, userID)
	r.heldLikes.release(r.likes, userID, func(id uuid.UUID) bool { return r.messages[id] != nil }, uncounted)

	return nil
}

func (r *TopicRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.heldLikes, userID)
	delete(r.hidden, userID)

	return nil
}

// Hold keeps aside the likes a deleted page put on photos of albums and
// on their comments.
func (r *AlbumRepository) Hold(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.hidden[userID] = true
	r.heldPhotoLikes.hold(r.photoLikes, userID, func(id uuid.UUID) { r.recountPhoto(id, -1, 0) })
	r.heldCommentLikes.hold(r.commentLikes, userID, uncounted)

	return nil
}

func (r *AlbumRepository) Release(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.hidden, userID)
	r.heldPhotoLikes.release(r.photoLikes, userID,
		func(id uuid.UUID) bool { return r.photos[id] != nil },
		func(id uuid.UUID) { r.recountPhoto(id, 1, 0) })
	r.heldCommentLikes.release(r.commentLikes, userID, func(id uuid.UUID) bool { return r.photoComments[id] != nil }, uncounted)

	return nil
}

func (r *AlbumRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.heldPhotoLikes, userID)
	delete(r.heldCommentLikes, userID)
	delete(r.hidden, userID)

	return nil
}
