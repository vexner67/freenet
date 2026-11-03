package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
)

func TestComments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	f.meduzaPage(channelPost("1", "news", f.now.Add(-time.Hour)))

	if _, err := f.svc.Subscribe(ctx, f.user, "@meduzalive"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	p, err := f.news.ByURI(ctx, "https://t.me/meduzalive/1")
	if err != nil {
		t.Fatalf("ByURI: %v", err)
	}

	other := uuid.New()
	first, err := f.svc.Comment(ctx, f.user, p.ID(), "  первый  ")
	if err != nil || first.Text() != "первый" || first.AuthorID() != f.user {
		t.Fatalf("Comment = %+v, %v", first, err)
	}
	if _, err = f.svc.Comment(ctx, other, p.ID(), "второй"); err != nil {
		t.Fatalf("Comment: %v", err)
	}

	page, err := f.svc.Comments(ctx, p.ID(), uuid.Nil, 1)
	if err != nil || page.Total != 2 || len(page.Comments) != 1 || page.Comments[0].ID() != first.ID() || page.Next == uuid.Nil {
		t.Fatalf("Comments = %+v, %v; want the oldest first", page, err)
	}
	if page, err = f.svc.Comments(ctx, p.ID(), page.Next, 1); err != nil || len(page.Comments) != 1 || page.Comments[0].AuthorID() != other || page.Next != uuid.Nil {
		t.Fatalf("Comments after the cursor = %+v, %v", page, err)
	}

	one, err := f.svc.NewsPost(ctx, p.ID(), f.user)
	if err != nil || one.Comments != 2 || one.Author == nil || one.Post.ID() != p.ID() {
		t.Fatalf("NewsPost = %+v, %v", one, err)
	}
	posts, err := f.svc.News(ctx, service.NewsQuery{Reader: f.user, Limit: 10})
	if err != nil || len(posts) != 1 || posts[0].Comments != 2 {
		t.Fatalf("News = %+v, %v; want 2 comments", posts, err)
	}

	if err = f.svc.DeleteComment(ctx, other, first.ID()); !errors.Is(err, fedi.ErrNotYourComment) {
		t.Fatalf("DeleteComment of another = %v", err)
	}
	if err = f.svc.DeleteComment(ctx, f.user, first.ID()); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if err = f.svc.DeleteComment(ctx, f.user, first.ID()); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("DeleteComment twice = %v", err)
	}

	if _, err = f.svc.Comment(ctx, f.user, p.ID(), "   "); !errors.Is(err, fedi.ErrCommentText) {
		t.Fatalf("Comment of nothing = %v", err)
	}
	if _, err = f.svc.Comment(ctx, f.user, uuid.New(), "мимо"); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Comment of no post = %v", err)
	}
	if _, err = f.svc.Comments(ctx, uuid.New(), uuid.Nil, 10); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Comments of no post = %v", err)
	}
	if _, err = f.svc.NewsPost(ctx, uuid.New(), f.user); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("NewsPost of no post = %v", err)
	}

	f.limiter.left = 0
	if _, err = f.svc.Comment(ctx, f.user, p.ID(), "ещё"); !errors.Is(err, fedi.ErrTooManyComments) {
		t.Fatalf("Comment over the limit = %v", err)
	}
}
