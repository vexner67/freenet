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

// WallPosts finds the posts of a wall, as PostRepository.ListByWall.
type WallPosts interface {
	ListByWall(ctx context.Context, wallID uuid.UUID, pick WallPick, before uuid.UUID, limit int) ([]*post.Post, error)
}

// PostDeleter deletes a post as requesterID would, as PostService.Delete.
type PostDeleter interface {
	Delete(ctx context.Context, requesterID, postID uuid.UUID) error
}

// maxLinkingPosts bounds the posts an article takes away with it.
const maxLinkingPosts = 20

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
	posts    WallPosts
	eraser   PostDeleter
	now      func() time.Time
}

// ArticleDeps are what ArticleService works with; Pages may be nil, when
// no page is ever deleted.
type ArticleDeps struct {
	Articles ArticleRepository
	Tx       TxManager
	Photos   PhotoKeeper
	Pages    Pages
	Posts    WallPosts
	Eraser   PostDeleter
}

func NewArticleService(d ArticleDeps) *ArticleService {
	return &ArticleService{articles: d.Articles, tx: d.Tx, photos: d.Photos, pages: d.Pages, posts: d.Posts, eraser: d.Eraser, now: time.Now}
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

// Delete deletes an article of its author with its photos and the posts
// of the author's wall that are its link alone; other posts that link to
// it say it is deleted.
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

		if err := s.dropPosts(ctx, a); err != nil {
			return errs.Wrap("s.dropPosts", err)
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

// dropPosts deletes the posts of the author's wall, archived or not, that
// are the link to a alone.
func (s *ArticleService) dropPosts(ctx context.Context, a *post.Article) error {
	query, err := post.NewWallQuery("/article/" + a.ID().String())
	if err != nil {
		return errs.Wrap("post.NewWallQuery", err)
	}

	for _, filter := range []post.WallFilter{post.WallOwner, post.WallArchive} {
		found, err := s.posts.ListByWall(ctx, a.AuthorID(), WallPick{Filter: filter, Query: query}, uuid.Nil, maxLinkingPosts)
		if err != nil {
			return errs.Wrap("s.posts.ListByWall", err)
		}

		for _, p := range found {
			if !a.LinkedAlone(p) {
				continue
			}

			if err := s.eraser.Delete(ctx, a.AuthorID(), p.ID()); err != nil {
				return errs.Wrap("s.eraser.Delete", err)
			}
		}
	}

	return nil
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
