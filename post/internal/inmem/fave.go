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

// faves pages likes, kept as what → users → when, from the side of
// userID, as the likes tables are read: newest first, by the time and
// then the ID, after the cursor. keep leaves out what is not to be shown.
func faves(likes map[uuid.UUID]map[uuid.UUID]time.Time, userID uuid.UUID, keep func(id uuid.UUID) bool, after *service.FaveCursor, limit int) []service.Fave {
	newer := func(a, b service.Fave) int {
		if c := b.LikedAt.Compare(a.LikedAt); c != 0 {
			return c
		}
		return bytes.Compare(b.ID[:], a.ID[:])
	}

	var all []service.Fave
	for id, users := range likes {
		at, ok := users[userID]
		if !ok || !keep(id) {
			continue
		}

		f := service.Fave{ID: id, LikedAt: at}
		if after == nil || newer(f, service.Fave{ID: after.ID, LikedAt: after.LikedAt}) > 0 {
			all = append(all, f)
		}
	}

	slices.SortFunc(all, newer)

	return all[:min(len(all), limit)]
}

// everyFave is a page no list of likes fills.
const everyFave = int(^uint(0) >> 1)

func everything(uuid.UUID) bool { return true }

// shownPost: the post is there and not archived. The caller holds the lock.
func (s *Store) shownPost(id uuid.UUID) bool {
	p, ok := s.posts[id]

	return ok && !p.Archived()
}

func (s *Store) shownComment(id uuid.UUID) bool {
	c, ok := s.comments[id]

	return ok && s.shownPost(c.PostID())
}

func (r *LikeRepository) LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return faves(r.s.likes, userID, r.s.shownPost, after, limit), nil
}

// CountLikedBy counts every like of userID, of what is in the archive too.
func (r *LikeRepository) CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return len(faves(r.s.likes, userID, everything, nil, everyFave)), nil
}

func (r *CommentLikeRepository) LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return faves(r.s.commentLikes, userID, r.s.shownComment, after, limit), nil
}

func (r *CommentLikeRepository) CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return len(faves(r.s.commentLikes, userID, everything, nil, everyFave)), nil
}

func (l *AlbumPhotoLikeRepository) LikedBy(ctx context.Context, userID uuid.UUID, after *service.FaveCursor, limit int) ([]service.Fave, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	l.r.mu.RLock()
	defer l.r.mu.RUnlock()

	return faves(l.r.photoLikes, userID, everything, after, limit), nil
}

func (l *AlbumPhotoLikeRepository) CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error) {
	all, err := l.LikedBy(ctx, userID, nil, everyFave)

	return len(all), err
}

// GetMany returns the photos that exist, in no order.
func (p *AlbumPhotoRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.AlbumPhoto, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	p.r.mu.RLock()
	defer p.r.mu.RUnlock()

	found := make([]*post.AlbumPhoto, 0, len(ids))
	for _, id := range ids {
		if photo, ok := p.r.photos[id]; ok {
			found = append(found, photo)
		}
	}

	return found, nil
}
