package inmem

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

type CommentLikeRepository struct {
	s *Store
}

func (r *CommentLikeRepository) Like(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	return r.set(ctx, commentID, userID, true)
}

func (r *CommentLikeRepository) Unlike(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	return r.set(ctx, commentID, userID, false)
}

// set likes or unlikes, counting a change only, and returns the count.
func (r *CommentLikeRepository) set(ctx context.Context, commentID, userID uuid.UUID, like bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.comments[commentID]; !ok {
		return 0, post.ErrCommentNotFound
	}

	users := r.s.commentLikes[commentID]
	if users == nil {
		users = make(map[uuid.UUID]time.Time)
		r.s.commentLikes[commentID] = users
	}

	_, liked := users[userID]

	switch {
	case like && !liked:
		users[userID] = time.Now().UTC().Truncate(time.Microsecond) // as PostgreSQL keeps it
		r.s.restoreComment(commentID, func(sc *post.StoredComment) { sc.Likes++ })
	case !like && liked:
		delete(users, userID)
		r.s.restoreComment(commentID, func(sc *post.StoredComment) { sc.Likes-- })
	}

	return r.s.comments[commentID].Likes(), nil
}

func (r *CommentLikeRepository) Liked(ctx context.Context, userID uuid.UUID, commentIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	liked := make(map[uuid.UUID]bool)
	for _, id := range commentIDs {
		if _, ok := r.s.commentLikes[id][userID]; ok {
			liked[id] = true
		}
	}

	return liked, nil
}

func (r *CommentLikeRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for commentID, users := range r.s.commentLikes {
		if _, ok := users[userID]; ok {
			delete(users, userID)
			r.s.restoreComment(commentID, func(sc *post.StoredComment) { sc.Likes-- })
		}
	}

	return nil
}

func (r *CommentLikeRepository) Likers(ctx context.Context, commentID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return likers(r.s.commentLikes[commentID], after, limit), nil
}
