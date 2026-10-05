package inmem

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type LikeRepository struct {
	s *Store
}

func (r *LikeRepository) Like(ctx context.Context, postID, userID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	p, ok := r.s.posts[postID]
	if !ok {
		return 0, post.ErrNotFound
	}

	users := r.s.likes[postID]
	if users == nil {
		users = make(map[uuid.UUID]struct{})
		r.s.likes[postID] = users
	}

	if _, ok = users[userID]; ok {
		return p.Counters().Likes, nil
	}

	users[userID] = struct{}{}
	r.s.count(postID, func(c *post.Counters) { c.Likes++ })

	return r.s.posts[postID].Counters().Likes, nil
}

func (r *LikeRepository) Unlike(ctx context.Context, postID, userID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	p, ok := r.s.posts[postID]
	if !ok {
		return 0, post.ErrNotFound
	}

	if _, ok = r.s.likes[postID][userID]; !ok {
		return p.Counters().Likes, nil
	}

	delete(r.s.likes[postID], userID)
	r.s.count(postID, func(c *post.Counters) { c.Likes-- })

	return r.s.posts[postID].Counters().Likes, nil
}

func (r *LikeRepository) Liked(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	liked := make(map[uuid.UUID]bool)
	for _, id := range postIDs {
		if _, ok := r.s.likes[id][userID]; ok {
			liked[id] = true
		}
	}

	return liked, nil
}
