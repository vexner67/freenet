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

// AlbumPhotoLikeRepository is the service.AlbumPhotoLikeRepository side
// of AlbumRepository.
type AlbumPhotoLikeRepository struct {
	r *AlbumRepository
}

func (r *AlbumRepository) PhotoLikes() *AlbumPhotoLikeRepository {
	return &AlbumPhotoLikeRepository{r: r}
}

func (l *AlbumPhotoLikeRepository) Like(ctx context.Context, photoID, userID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	l.r.mu.Lock()
	defer l.r.mu.Unlock()

	if _, ok := l.r.photos[photoID]; !ok {
		return 0, post.ErrAlbumPhotoNotFound
	}

	users := l.r.photoLikes[photoID]
	if users == nil {
		users = make(map[uuid.UUID]time.Time)
		l.r.photoLikes[photoID] = users
	}

	if _, ok := users[userID]; !ok {
		users[userID] = time.Now().UTC().Truncate(time.Microsecond) // as PostgreSQL keeps it
		l.r.recountPhoto(photoID, 1, 0)
	}

	return l.r.photos[photoID].Likes(), nil
}

func (l *AlbumPhotoLikeRepository) Unlike(ctx context.Context, photoID, userID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	l.r.mu.Lock()
	defer l.r.mu.Unlock()

	if _, ok := l.r.photos[photoID]; !ok {
		return 0, post.ErrAlbumPhotoNotFound
	}

	if _, ok := l.r.photoLikes[photoID][userID]; ok {
		delete(l.r.photoLikes[photoID], userID)
		l.r.recountPhoto(photoID, -1, 0)
	}

	return l.r.photos[photoID].Likes(), nil
}

func (l *AlbumPhotoLikeRepository) Liked(ctx context.Context, userID uuid.UUID, photoIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	l.r.mu.RLock()
	defer l.r.mu.RUnlock()

	liked := make(map[uuid.UUID]bool)
	for _, id := range photoIDs {
		if _, ok := l.r.photoLikes[id][userID]; ok {
			liked[id] = true
		}
	}

	return liked, nil
}

func (l *AlbumPhotoLikeRepository) Likers(ctx context.Context, photoID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	l.r.mu.RLock()
	defer l.r.mu.RUnlock()

	return likers(l.r.photoLikes[photoID], after, limit), nil
}

// PhotoCommentRepository is the service.PhotoCommentRepository side of
// AlbumRepository.
type PhotoCommentRepository struct {
	r *AlbumRepository
}

func (r *AlbumRepository) PhotoComments() *PhotoCommentRepository {
	return &PhotoCommentRepository{r: r}
}

func (c *PhotoCommentRepository) Save(ctx context.Context, pc *post.PhotoComment) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	c.r.mu.Lock()
	defer c.r.mu.Unlock()

	if _, ok := c.r.photos[pc.PhotoID()]; !ok {
		return post.ErrAlbumPhotoNotFound
	}

	if _, ok := c.r.photoComments[pc.ReplyTo()]; pc.ReplyTo() != uuid.Nil && !ok {
		return post.ErrCommentNotFound
	}

	c.r.photoComments[pc.ID()] = pc
	c.r.recountPhoto(pc.PhotoID(), 0, 1)

	return nil
}

func (c *PhotoCommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.PhotoComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	pc, ok := c.r.photoComments[id]
	if !ok {
		return nil, post.ErrCommentNotFound
	}

	return c.r.counted(pc), nil
}

func (c *PhotoCommentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.PhotoComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	var found []*post.PhotoComment
	for _, id := range ids {
		if pc, ok := c.r.photoComments[id]; ok {
			found = append(found, c.r.counted(pc))
		}
	}

	return found, nil
}

func (c *PhotoCommentRepository) Like(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	return c.setLike(ctx, commentID, userID, true)
}

func (c *PhotoCommentRepository) Unlike(ctx context.Context, commentID, userID uuid.UUID) (int, error) {
	return c.setLike(ctx, commentID, userID, false)
}

func (c *PhotoCommentRepository) setLike(ctx context.Context, commentID, userID uuid.UUID, like bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.Lock()
	defer c.r.mu.Unlock()

	if _, ok := c.r.photoComments[commentID]; !ok {
		return 0, post.ErrCommentNotFound
	}

	if c.r.commentLikes[commentID] == nil {
		c.r.commentLikes[commentID] = make(map[uuid.UUID]time.Time)
	}

	if _, liked := c.r.commentLikes[commentID][userID]; like && !liked {
		c.r.commentLikes[commentID][userID] = time.Now()
	} else if !like {
		delete(c.r.commentLikes[commentID], userID)
	}

	return len(c.r.commentLikes[commentID]), nil
}

func (c *PhotoCommentRepository) Liked(ctx context.Context, userID uuid.UUID, commentIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	liked := make(map[uuid.UUID]bool)
	for _, id := range commentIDs {
		if _, ok := c.r.commentLikes[id][userID]; ok {
			liked[id] = true
		}
	}

	return liked, nil
}

func (c *PhotoCommentRepository) Likers(ctx context.Context, commentID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	return likers(c.r.commentLikes[commentID], after, limit), nil
}

func (c *PhotoCommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	c.r.mu.Lock()
	defer c.r.mu.Unlock()

	pc, ok := c.r.photoComments[id]
	if !ok {
		return post.ErrCommentNotFound
	}

	c.r.deleteComment(id)
	c.r.recountPhoto(pc.PhotoID(), 0, -1)

	return nil
}

func (c *PhotoCommentRepository) List(ctx context.Context, photoID, after uuid.UUID, limit int) ([]*post.PhotoComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	var found []*post.PhotoComment
	for _, pc := range c.r.photoComments {
		if id := pc.ID(); pc.PhotoID() == photoID && bytes.Compare(id[:], after[:]) > 0 && !c.r.hidden[pc.AuthorID()] {
			found = append(found, c.r.counted(pc))
		}
	}

	slices.SortFunc(found, func(a, b *post.PhotoComment) int {
		x, y := a.ID(), b.ID()
		return bytes.Compare(x[:], y[:])
	})

	return found[:min(len(found), limit)], nil
}
