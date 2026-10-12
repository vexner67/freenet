package inmem

import (
	"bytes"
	"context"
	"math"
	"slices"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type CommentRepository struct {
	s *Store
}

// Comments are immutable, so sharing pointers with callers is safe.
func (r *CommentRepository) Save(ctx context.Context, c *post.Comment) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.posts[c.PostID()]; !ok {
		return post.ErrNotFound
	}

	r.s.comments[c.ID()] = c
	r.s.count(c.PostID(), func(c *post.Counters) { c.Comments++ })

	return nil
}

func (r *CommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.Comment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	c, ok := r.s.comments[id]
	if !ok {
		return nil, post.ErrCommentNotFound
	}

	return c, nil
}

func (r *CommentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Comment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*post.Comment
	for _, id := range ids {
		if c, ok := r.s.comments[id]; ok {
			found = append(found, c)
		}
	}

	return found, nil
}

func (r *CommentRepository) LastByPosts(ctx context.Context, postIDs []uuid.UUID, limit int) ([]*post.Comment, error) {
	var found []*post.Comment

	for _, postID := range postIDs {
		r.s.mu.RLock()
		p, ok := r.s.posts[postID]
		r.s.mu.RUnlock()

		if !ok || p.Archived() {
			continue
		}

		// Oldest first: the last of them are the newest.
		comments, err := r.ListByPost(ctx, postID, uuid.Nil, math.MaxInt)
		if err != nil {
			return nil, err
		}

		found = append(found, comments[max(0, len(comments)-limit):]...)
	}

	return found, nil
}

func (r *CommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	c, ok := r.s.comments[id]
	if !ok {
		return post.ErrCommentNotFound
	}

	r.s.removeComment(id)
	r.s.count(c.PostID(), func(c *post.Counters) { c.Comments-- })

	return nil
}

func (r *CommentRepository) ListByPost(ctx context.Context, postID, after uuid.UUID, limit int) ([]*post.Comment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*post.Comment
	for _, c := range r.s.comments {
		id := c.ID()
		if c.PostID() == postID && bytes.Compare(id[:], after[:]) > 0 && !r.s.commentHidden(c) {
			found = append(found, c)
		}
	}

	// Oldest first: UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(a, b *post.Comment) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(ai[:], bi[:])
	})

	if len(found) > limit {
		found = found[:limit]
	}

	return found, nil
}

func (r *CommentRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for id, c := range r.s.comments {
		if c.AuthorID() == userID {
			r.s.removeComment(id)
			r.s.count(c.PostID(), func(c *post.Counters) { c.Comments-- })
		}
	}

	return nil
}

// DeleteByPost deletes the comments of a post about to be deleted,
// returning them; the post's counter is left alone.
func (r *CommentRepository) DeleteByPost(ctx context.Context, postID uuid.UUID) ([]*post.Comment, error) {
	return r.deleteWhere(ctx, func(c *post.Comment) bool { return c.PostID() == postID })
}

// DeleteOnPostsOf deletes the comments on the posts a user wrote and on
// those on their wall, returning them.
func (r *CommentRepository) DeleteOnPostsOf(ctx context.Context, userID uuid.UUID) ([]*post.Comment, error) {
	return r.deleteWhere(ctx, func(c *post.Comment) bool {
		p := r.s.posts[c.PostID()]
		return p != nil && (p.AuthorID() == userID || p.WallID() == userID)
	})
}

func (r *CommentRepository) deleteWhere(ctx context.Context, match func(*post.Comment) bool) ([]*post.Comment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	var deleted []*post.Comment
	for id, c := range r.s.comments {
		if match(c) {
			r.s.removeComment(id)
			deleted = append(deleted, c)
		}
	}

	return deleted, nil
}
