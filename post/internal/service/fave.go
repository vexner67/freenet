package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// Fave is something a person likes, and since when.
type Fave struct {
	ID      uuid.UUID
	LikedAt time.Time
}

// FaveCursor is where a page of faves ends: they are read newest first,
// by the time of the like and then the ID of what is liked.
type FaveCursor struct {
	LikedAt time.Time
	ID      uuid.UUID
}

// FaveRepository reads the likes of one kind (of posts, of comments, of
// photos of albums) from the side of who likes. What sits on an archived
// post is left out of both: only the owner of its wall reads it.
type FaveRepository interface {
	// LikedBy returns up to limit things userID likes, newest like first,
	// after the cursor (nil: from the newest).
	LikedBy(ctx context.Context, userID uuid.UUID, after *FaveCursor, limit int) ([]Fave, error)
	CountLikedBy(ctx context.Context, userID uuid.UUID) (int, error)
}

// FavePosts, FaveComments and FavePhotos read what is liked, in no order;
// what is gone meanwhile is missing.
type FavePosts interface {
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error)
}

type FaveComments interface {
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Comment, error)
}

type FavePhotos interface {
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.AlbumPhoto, error)
}

// FaveAlbums tells whose album a liked photo is in.
type FaveAlbums interface {
	Get(ctx context.Context, id uuid.UUID) (*post.Album, error)
}

type FaveDeps struct {
	PostLikes    FaveRepository
	CommentLikes FaveRepository
	PhotoLikes   FaveRepository
	Posts        FavePosts
	Comments     FaveComments
	Photos       FavePhotos
	Albums       FaveAlbums
	// Walls tells the walls closed to a viewer; nil: none is.
	Walls Walls
}

// FaveService is openvk's "Закладки": what a person likes — posts,
// comments and photos of albums — newest like first, for them alone.
type FaveService struct {
	deps FaveDeps
}

func NewFaveService(deps FaveDeps) *FaveService {
	return &FaveService{deps: deps}
}

// FavePostPage is one screen of liked posts; Total counts them all, Next
// is nil on the last page.
type FavePostPage struct {
	Posts []*post.Post
	Next  *FaveCursor
	Total int
}

type FaveCommentPage struct {
	Comments []*post.Comment
	Next     *FaveCursor
	Total    int
}

// FavePhotoPage: Owners tells whose album each photo is in, by the ID of
// the album — who may see the photo is theirs to say.
type FavePhotoPage struct {
	Photos []*post.AlbumPhoto
	Owners map[uuid.UUID]uuid.UUID
	Next   *FaveCursor
	Total  int
}

// Posts pages the posts userID likes.
func (s *FaveService) Posts(ctx context.Context, userID uuid.UUID, cursor *FaveCursor, size int) (FavePostPage, error) {
	faves, next, total, err := s.page(ctx, s.deps.PostLikes, userID, cursor, size)
	if err != nil {
		return FavePostPage{}, errs.Wrap("s.page", err)
	}

	posts, err := s.deps.Posts.GetMany(ctx, faveIDs(faves))
	if err != nil {
		return FavePostPage{}, errs.Wrap("s.deps.Posts.GetMany", err)
	}

	// A post on a wall since closed to the reader is counted and not shown.
	if posts, err = readable(ctx, s.deps.Walls, userID, posts); err != nil {
		return FavePostPage{}, errs.Wrap("readable", err)
	}

	return FavePostPage{Posts: inFaveOrder(faves, posts, (*post.Post).ID), Next: next, Total: total}, nil
}

// Comments pages the comments under posts userID likes.
func (s *FaveService) Comments(ctx context.Context, userID uuid.UUID, cursor *FaveCursor, size int) (FaveCommentPage, error) {
	faves, next, total, err := s.page(ctx, s.deps.CommentLikes, userID, cursor, size)
	if err != nil {
		return FaveCommentPage{}, errs.Wrap("s.page", err)
	}

	comments, err := s.deps.Comments.GetMany(ctx, faveIDs(faves))
	if err != nil {
		return FaveCommentPage{}, errs.Wrap("s.deps.Comments.GetMany", err)
	}

	return FaveCommentPage{Comments: inFaveOrder(faves, comments, (*post.Comment).ID), Next: next, Total: total}, nil
}

// Photos pages the photos of albums userID likes.
func (s *FaveService) Photos(ctx context.Context, userID uuid.UUID, cursor *FaveCursor, size int) (FavePhotoPage, error) {
	faves, next, total, err := s.page(ctx, s.deps.PhotoLikes, userID, cursor, size)
	if err != nil {
		return FavePhotoPage{}, errs.Wrap("s.page", err)
	}

	photos, err := s.deps.Photos.GetMany(ctx, faveIDs(faves))
	if err != nil {
		return FavePhotoPage{}, errs.Wrap("s.deps.Photos.GetMany", err)
	}

	page := FavePhotoPage{Next: next, Total: total, Owners: make(map[uuid.UUID]uuid.UUID)}

	for _, p := range inFaveOrder(faves, photos, (*post.AlbumPhoto).ID) {
		if _, ok := page.Owners[p.AlbumID()]; !ok {
			a, err := s.deps.Albums.Get(ctx, p.AlbumID())
			if err != nil {
				return FavePhotoPage{}, errs.Wrap("s.deps.Albums.Get", err)
			}

			page.Owners[p.AlbumID()] = a.OwnerID()
		}

		page.Photos = append(page.Photos, p)
	}

	return page, nil
}

// page reads one screen of the likes of userID and counts them all.
func (s *FaveService) page(ctx context.Context, likes FaveRepository, userID uuid.UUID, cursor *FaveCursor, size int) ([]Fave, *FaveCursor, int, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	faves, err := likes.LikedBy(ctx, userID, cursor, size+1)
	if err != nil {
		return nil, nil, 0, errs.Wrap("likes.LikedBy", err)
	}

	total, err := likes.CountLikedBy(ctx, userID)
	if err != nil {
		return nil, nil, 0, errs.Wrap("likes.CountLikedBy", err)
	}

	var next *FaveCursor
	if len(faves) > size {
		faves = faves[:size]
		last := faves[size-1]
		next = &FaveCursor{LikedAt: last.LikedAt, ID: last.ID}
	}

	return faves, next, total, nil
}

func faveIDs(faves []Fave) []uuid.UUID {
	ids := make([]uuid.UUID, len(faves))
	for i, f := range faves {
		ids[i] = f.ID
	}

	return ids
}

// inFaveOrder puts found, read in no order, as faves list them; what was
// not found is skipped.
func inFaveOrder[T any](faves []Fave, found []T, id func(T) uuid.UUID) []T {
	byID := make(map[uuid.UUID]T, len(found))
	for _, f := range found {
		byID[id(f)] = f
	}

	ordered := make([]T, 0, len(found))
	for _, f := range faves {
		if v, ok := byID[f.ID]; ok {
			ordered = append(ordered, v)
		}
	}

	return ordered
}
