package inmem

import (
	"bytes"
	"context"
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

	delete(r.s.comments, id)
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
		if c.PostID() == postID && bytes.Compare(id[:], after[:]) > 0 {
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
