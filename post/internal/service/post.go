package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 50

	// MaxFeedAuthors bounds one news feed: the people a user follows plus
	// the user.
	MaxFeedAuthors = 1001
)

var ErrTooManyAuthors = errors.New("too many authors")

type PostRepository interface {
	Save(ctx context.Context, p *post.Post) error
	// Get and Delete return post.ErrNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Post, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// ListByAuthor returns up to limit posts, newest first, with IDs below
	// before (uuid.Nil: from the newest).
	ListByAuthor(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Post, error)
	CountByAuthor(ctx context.Context, authorID uuid.UUID) (int, error)
	// ListByAuthors is ListByAuthor for several authors at once.
	ListByAuthors(ctx context.Context, authorIDs []uuid.UUID, before uuid.UUID, limit int) ([]*post.Post, error)
}

// Page is one screen of a wall. NextCursor is uuid.Nil on the last page.
type Page struct {
	Posts      []*post.Post
	NextCursor uuid.UUID
	Total      int
}

type PostService struct {
	postRepo PostRepository
	now      func() time.Time
}

func NewPostService(postRepo PostRepository) *PostService {
	return &PostService{
		postRepo: postRepo,
		now:      time.Now,
	}
}

func (s *PostService) Create(ctx context.Context, authorID uuid.UUID, text post.Text) (*post.Post, error) {
	p, err := post.New(authorID, text, s.now())
	if err != nil {
		return nil, errs.Wrap("post.New", err)
	}

	if err = s.postRepo.Save(ctx, p); err != nil {
		return nil, errs.Wrap("s.postRepo.Save", err)
	}

	return p, nil
}

func (s *PostService) Delete(ctx context.Context, requesterID, postID uuid.UUID) error {
	p, err := s.postRepo.Get(ctx, postID)
	if err != nil {
		return errs.Wrap("s.postRepo.Get", err)
	}

	if !p.CanBeDeletedBy(requesterID) {
		return post.ErrNotAuthor
	}

	if err = s.postRepo.Delete(ctx, postID); err != nil {
		return errs.Wrap("s.postRepo.Delete", err)
	}

	return nil
}

// ListByAuthor pages a wall by keyset: cursor is the ID of the last post
// of the previous page, so new posts never shift the pages.
func (s *PostService) ListByAuthor(ctx context.Context, authorID, cursor uuid.UUID, size int) (Page, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra post tells whether there is a next page.
	posts, err := s.postRepo.ListByAuthor(ctx, authorID, cursor, size+1)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.ListByAuthor", err)
	}

	total, err := s.postRepo.CountByAuthor(ctx, authorID)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.CountByAuthor", err)
	}

	page := Page{Posts: posts, Total: total}

	if len(posts) > size {
		page.Posts = posts[:size]
		page.NextCursor = posts[size-1].ID()
	}

	return page, nil
}

// Feed pages the posts of several authors together, newest first. Total
// is not counted: a feed is read page by page, not as a whole.
func (s *PostService) Feed(ctx context.Context, authorIDs []uuid.UUID, cursor uuid.UUID, size int) (Page, error) {
	if len(authorIDs) > MaxFeedAuthors {
		return Page{}, ErrTooManyAuthors
	}

	if len(authorIDs) == 0 {
		return Page{}, nil
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	posts, err := s.postRepo.ListByAuthors(ctx, authorIDs, cursor, size+1)
	if err != nil {
		return Page{}, errs.Wrap("s.postRepo.ListByAuthors", err)
	}

	page := Page{Posts: posts}

	if len(posts) > size {
		page.Posts = posts[:size]
		page.NextCursor = posts[size-1].ID()
	}

	return page, nil
}
