package service_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestPostService_NSFW(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	svc, _ := newWallServices(uuid.New(), uuid.New())

	plain := write(t, svc, ann, "котики")[0]
	if plain.NSFW() {
		t.Fatal("a post is marked without being asked")
	}

	adult, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: ann, Content: textContent(t, "для взрослых"), NSFW: true})
	if err != nil || !adult.NSFW() {
		t.Fatalf("Create = %+v, %v", adult, err)
	}
	if got, _, err := svc.Get(ctx, uuid.Nil, adult.ID()); err != nil || !got.NSFW() {
		t.Fatalf("the saved post = %+v, %v", got, err)
	}

	// The news show it, unless their reader asked not to.
	sources := service.FeedSources{Authors: []uuid.UUID{ann}}
	if feed, err := svc.Feed(ctx, ann, sources, uuid.Nil, 10); err != nil || !slices.Equal(texts(feed.Posts), []string{"для взрослых", "котики"}) {
		t.Fatalf("the feed = %v, %v", texts(feed.Posts), err)
	}
	sources.HideNSFW = true
	if feed, err := svc.Feed(ctx, ann, sources, uuid.Nil, 10); err != nil || !slices.Equal(texts(feed.Posts), []string{"котики"}) {
		t.Fatalf("the feed without adult posts = %v, %v", texts(feed.Posts), err)
	}
	// The wall shows it to everyone: blurring is the site's.
	if wall, err := svc.ListWall(ctx, uuid.Nil, ann, service.WallPick{}, uuid.Nil, 10); err != nil || wall.Total != 2 {
		t.Fatalf("the wall = %d, %v", wall.Total, err)
	}
	// "Все новости" follow nobody: they show what the walls say themselves,
	// not what a guest wrote there.
	bob := uuid.New()
	if _, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: textContent(t, "в гостях")}); err != nil {
		t.Fatal(err)
	}
	everyone := service.FeedSources{Everyone: true}
	if feed, err := svc.Feed(ctx, bob, everyone, uuid.Nil, 10); err != nil || !slices.Equal(texts(feed.Posts), []string{"для взрослых", "котики"}) {
		t.Fatalf("the feed of everyone = %v, %v", texts(feed.Posts), err)
	}
	everyone.HideNSFW = true
	if feed, err := svc.Feed(ctx, bob, everyone, uuid.Nil, 10); err != nil || !slices.Equal(texts(feed.Posts), []string{"котики"}) {
		t.Fatalf("the feed of everyone without adult posts = %v, %v", texts(feed.Posts), err)
	}

	// An edit that says nothing of the mark keeps it; one that does changes it.
	edited, _, err := svc.Edit(ctx, ann, adult.ID(), service.PostEdit{Text: "всё ещё для взрослых"})
	if err != nil || !edited.NSFW() {
		t.Fatalf("an edit of the text = %+v, %v", edited, err)
	}

	off, on := false, true
	if edited, _, err = svc.Edit(ctx, ann, adult.ID(), service.PostEdit{Text: "для всех", NSFW: &off}); err != nil || edited.NSFW() {
		t.Fatalf("the mark taken off = %+v, %v", edited, err)
	}
	if got, _, _ := svc.Get(ctx, uuid.Nil, adult.ID()); got.NSFW() || got.Content().Text() != post.Text("для всех") {
		t.Fatalf("the saved edit = %+v", got)
	}
	if edited, _, err = svc.Edit(ctx, ann, plain.ID(), service.PostEdit{Text: "котики", NSFW: &on}); err != nil || !edited.NSFW() {
		t.Fatalf("the mark put on = %+v, %v", edited, err)
	}
}
