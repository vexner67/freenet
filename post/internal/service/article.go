package service

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type ArticleRepository interface {
	Save(ctx context.Context, a *post.Article) error
	// Get, Update and Delete return post.ErrArticleNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Article, error)
	Update(ctx context.Context, a *post.Article) error
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit articles of an author, newest first, with
	// IDs below before (uuid.Nil: from the newest); Count counts them all.
	List(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Article, error)
	Count(ctx context.Context, authorID uuid.UUID) (int, error)
}

// ArticlePage is one screen of the articles of a person; NextCursor is
// uuid.Nil on the last, Total counts them on all pages.
type ArticlePage struct {
	Articles   []*post.Article
	NextCursor uuid.UUID
	Total      int
}

// ArticleService keeps the articles people write in the editor: media
// keeps their photos for good while the article shows them. Anybody reads
// an article; only its author changes it. What a deleted page wrote is
// not found.
type ArticleService struct {
	articles ArticleRepository
	tx       TxManager
	photos   PhotoKeeper
	pages    Pages
	now      func() time.Time
}

// NewArticleService: pages may be nil, when no page is ever deleted.
func NewArticleService(articles ArticleRepository, tx TxManager, photos PhotoKeeper, pages Pages) *ArticleService {
	return &ArticleService{articles: articles, tx: tx, photos: photos, pages: pages, now: time.Now}
}

// Create writes an article of authorID. The caller has checked that the
// photos are the author's and claimed them.
func (s *ArticleService) Create(ctx context.Context, authorID uuid.UUID, d post.ArticleDraft) (*post.Article, error) {
	a, err := post.NewArticle(authorID, d, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewArticle", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.articles.Save(ctx, a); err != nil {
			return errs.Wrap("s.articles.Save", err)
		}

		return errs.Wrap("s.publish", s.publish(ctx, authorID, a.Photos()))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return a, nil
}

// Edit writes an article anew; media keeps its new photos and deletes
// those it no longer shows.
func (s *ArticleService) Edit(ctx context.Context, requesterID, id uuid.UUID, d post.ArticleDraft) (*post.Article, error) {
	a, err := s.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.Get", err)
	}

	edited, err := a.Edited(requesterID, d, s.now())
	if err != nil {
		return nil, errs.Wrap("a.Edited", err)
	}

	added, removed := diffPhotos(a.Photos(), edited.Photos())

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.articles.Update(ctx, edited); err != nil {
			return errs.Wrap("s.articles.Update", err)
		}

		if err := s.publish(ctx, a.AuthorID(), added); err != nil {
			return errs.Wrap("s.publish", err)
		}

		return errs.Wrap("s.drop", s.drop(ctx, a.AuthorID(), removed))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return edited, nil
}

// Delete deletes an article of its author with its photos; the posts that
// link to it show the link alone.
func (s *ArticleService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	a, err := s.articles.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.articles.Get", err)
	}

	if !a.CanBeChangedBy(requesterID) {
		return post.ErrCannotChangeArticle
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.articles.Delete(ctx, id); err != nil {
			return errs.Wrap("s.articles.Delete", err)
		}

		return errs.Wrap("s.drop", s.drop(ctx, a.AuthorID(), a.Photos()))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Get finds an article; that of a deleted page is not found.
func (s *ArticleService) Get(ctx context.Context, id uuid.UUID) (*post.Article, error) {
	a, err := s.articles.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.articles.Get", err)
	}

	gone, err := deletedPages(ctx, s.pages, a.AuthorID())
	if err != nil {
		return nil, errs.Wrap("deletedPages", err)
	}

	if gone[a.AuthorID()] {
		return nil, post.ErrArticleNotFound
	}

	return a, nil
}

// List pages the articles of authorID, newest first; a deleted page has
// none.
func (s *ArticleService) List(ctx context.Context, authorID, cursor uuid.UUID, size int) (ArticlePage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	gone, err := deletedPages(ctx, s.pages, authorID)
	if err != nil {
		return ArticlePage{}, errs.Wrap("deletedPages", err)
	}

	if gone[authorID] {
		return ArticlePage{}, nil
	}

	// One extra tells whether there is a next page.
	found, err := s.articles.List(ctx, authorID, cursor, size+1)
	if err != nil {
		return ArticlePage{}, errs.Wrap("s.articles.List", err)
	}

	page := ArticlePage{Articles: found}
	if len(found) > size {
		page.Articles = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Total, err = s.articles.Count(ctx, authorID); err != nil {
		return ArticlePage{}, errs.Wrap("s.articles.Count", err)
	}

	return page, nil
}

func (s *ArticleService) publish(ctx context.Context, ownerID uuid.UUID, photos []uuid.UUID) error {
	if len(photos) == 0 {
		return nil
	}

	return errs.Wrap("s.photos.Publish", s.photos.Publish(ctx, ownerID, slices.Clone(photos)))
}

func (s *ArticleService) drop(ctx context.Context, ownerID uuid.UUID, photos []uuid.UUID) error {
	if len(photos) == 0 {
		return nil
	}

	return errs.Wrap("s.photos.Delete", s.photos.Delete(ctx, ownerID, slices.Clone(photos)))
}
