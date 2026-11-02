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

func TestLikes(t *testing.T) {
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
	for _, user := range []uuid.UUID{f.user, other, f.user} {
		if _, err = f.svc.Like(ctx, user, p.ID()); err != nil {
			t.Fatalf("Like: %v", err)
		}
	}

	posts, err := f.svc.News(ctx, service.NewsQuery{Reader: f.user, Limit: 10})
	if err != nil || len(posts) != 1 || posts[0].Likes != (service.Likes{Count: 2, Liked: true}) {
		t.Fatalf("News = %+v, %v; want 2 likes, one of the reader", posts, err)
	}

	page, err := f.svc.Likers(ctx, p.ID(), uuid.Nil, 1)
	if err != nil || page.Total != 2 || len(page.UserIDs) != 1 || page.UserIDs[0] != other || page.Next == uuid.Nil {
		t.Fatalf("Likers = %+v, %v; want the latest first", page, err)
	}

	if page, err = f.svc.Likers(ctx, p.ID(), page.Next, 1); err != nil || len(page.UserIDs) != 1 || page.UserIDs[0] != f.user || page.Next != uuid.Nil {
		t.Fatalf("Likers after the cursor = %+v, %v", page, err)
	}

	if n, err := f.svc.Unlike(ctx, f.user, p.ID()); err != nil || n != 1 {
		t.Fatalf("Unlike = %d, %v", n, err)
	}

	if posts, _ = f.svc.News(ctx, service.NewsQuery{Reader: f.user, Limit: 10}); posts[0].Likes != (service.Likes{Count: 1}) {
		t.Fatalf("likes after Unlike = %+v", posts[0].Likes)
	}

	if _, err = f.svc.Like(ctx, f.user, uuid.New()); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Like of no post = %v", err)
	}
	if _, err = f.svc.Likers(ctx, uuid.New(), uuid.Nil, 10); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Likers of no post = %v", err)
	}

	f.limiter.left = 0
	if _, err = f.svc.Like(ctx, f.user, p.ID()); !errors.Is(err, fedi.ErrTooManyLikes) {
		t.Fatalf("Like over the limit = %v", err)
	}
	if _, err = f.svc.Unlike(ctx, f.user, p.ID()); !errors.Is(err, fedi.ErrTooManyLikes) {
		t.Fatalf("Unlike over the limit = %v", err)
	}
}
