package inmem

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
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
		users = make(map[uuid.UUID]time.Time)
		r.s.likes[postID] = users
	}

	if _, ok = users[userID]; ok {
		return p.Counters().Likes, nil
	}

	users[userID] = time.Now().UTC().Truncate(time.Microsecond) // as PostgreSQL keeps it
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

func (r *LikeRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for postID, users := range r.s.likes {
		if _, ok := users[userID]; ok {
			delete(users, userID)
			r.s.count(postID, func(c *post.Counters) { c.Likes-- })
		}
	}

	return nil
}

func (r *LikeRepository) Likers(ctx context.Context, postID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return likers(r.s.likes[postID], after, limit), nil
}

// likers pages users → when, as the likes tables are read: newest first,
// by the time and then the user, after the cursor.
func likers(users map[uuid.UUID]time.Time, after *service.LikerCursor, limit int) []service.Liker {
	newer := func(a, b service.Liker) int {
		if c := b.LikedAt.Compare(a.LikedAt); c != 0 {
			return c
		}
		return bytes.Compare(b.UserID[:], a.UserID[:])
	}

	all := make([]service.Liker, 0, len(users))
	for id, at := range users {
		l := service.Liker{UserID: id, LikedAt: at}
		if after == nil || newer(l, service.Liker{UserID: after.UserID, LikedAt: after.LikedAt}) > 0 {
			all = append(all, l)
		}
	}

	slices.SortFunc(all, newer)

	return all[:min(len(all), limit)]
}
