package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/postgres"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func TestNewsRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	following := postgres.NewFollowingRepository(pool)
	subscriptions := postgres.NewSubscriptionRepository(pool)
	news := postgres.NewRemotePostRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Actors of their own, so that runs do not meet.
	featured := fedi.URL("https://m.example/users/" + uuid.NewString())
	read := fedi.URL("https://m.example/users/" + uuid.NewString())
	reader := uuid.New()

	if _, err = following.Get(ctx, featured); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Get of no follow = %v", err)
	}

	f, err := fedi.NewFollowing(featured, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = following.Save(ctx, f); err != nil {
		t.Fatal(err)
	}
	if err = following.Save(ctx, f.WithAccepted(true)); err != nil {
		t.Fatal(err)
	}

	got, err := following.Get(ctx, featured)
	if err != nil || !got.Accepted() || !got.Featured() || got.FollowID() != f.FollowID() || !got.CreatedAt().Equal(now) {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	all, err := following.Featured(ctx)
	if err != nil || !containsFollow(all, featured) {
		t.Fatalf("Featured = %v, %v", all, err)
	}

	r, err := fedi.NewFollowing(read, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = following.Save(ctx, r); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err = subscriptions.Add(ctx, reader, read, now); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	mine, err := subscriptions.Of(ctx, reader)
	if err != nil || len(mine) != 1 || mine[0] != read {
		t.Fatalf("Of = %v, %v", mine, err)
	}

	if ok, err := subscriptions.Read(ctx, read); err != nil || !ok {
		t.Fatalf("Read = %v, %v", ok, err)
	}

	post := func(actor fedi.URL, n string, language string, sensitive bool, at time.Time) *fedi.RemotePost {
		doc := fedi.NoteDoc{
			ID: actor.String() + "/notes/" + n, AttributedTo: actor.String(), Text: n, Language: language,
			Public: true, Published: at,
			Attachments: []fedi.AttachmentDoc{
				{URL: "https://m.example/" + n + ".png", MediaType: "image/png", Description: "d"},
				{URL: "https://m.example/" + n + ".mp4", MediaType: "video/mp4", Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj", Width: 640, Height: 360, Duration: 90 * time.Second},
			},
		}
		if sensitive {
			doc.Summary = "cw"
		}

		p, err := fedi.NewRemotePost(doc, actor, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = news.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return p
	}

	at := now.Add(-time.Minute)
	en := post(featured, "en", "en", false, at)
	post(featured, "de", "de", false, at.Add(time.Second))
	cw := post(featured, "cw", "", true, at.Add(2*time.Second))
	mineDE := post(read, "mine", "de", false, at.Add(3*time.Second))

	byURI, err := news.ByURI(ctx, en.URI())
	if err != nil || byURI.ID() != en.ID() || len(byURI.Images()) != 1 || byURI.Images()[0].Description != "d" || !byURI.Published().Equal(at) {
		t.Fatalf("ByURI = %+v, %v", byURI, err)
	}

	if videos := byURI.Videos(); len(videos) != 1 || videos[0] != en.Videos()[0] {
		t.Fatalf("videos = %+v, want %+v", videos, en.Videos())
	}

	covers := postgres.NewCoverRepository(pool)
	video := en.Videos()[0].URL

	if _, err = covers.Get(ctx, en.ID(), video); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Get of a frame never cut = %v", err)
	}
	if err = covers.Save(ctx, en.ID(), video, nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if frame, err := covers.Get(ctx, en.ID(), video); err != nil || len(frame) != 0 {
		t.Fatalf("Get of no frame = %q, %v", frame, err)
	}
	if err = covers.Save(ctx, en.ID(), video, []byte("jpeg")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if frame, err := covers.Get(ctx, en.ID(), video); err != nil || string(frame) != "jpeg" {
		t.Fatalf("Get = %q, %v", frame, err)
	}
	if err = covers.Save(ctx, uuid.New(), video, []byte("jpeg")); err != nil {
		t.Fatalf("Save of a frame of no post: %v", err)
	}

	list := func(q service.NewsQuery) []uuid.UUID {
		t.Helper()

		q.Limit = 1000
		posts, err := news.List(ctx, q)
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		var ids []uuid.UUID
		for _, p := range posts {
			if p.Actor() == featured || p.Actor() == read {
				ids = append(ids, p.ID())
			}
		}

		return ids
	}

	if ids := list(service.NewsQuery{Featured: true, Languages: []string{"ru", "en"}}); !sameIDs(ids, cw.ID(), en.ID()) {
		t.Fatalf("featured = %v, want cw, en", ids)
	}

	if ids := list(service.NewsQuery{Featured: true, Languages: []string{"ru", "en"}, HideSensitive: true, Before: cw.ID()}); !sameIDs(ids, en.ID()) {
		t.Fatalf("featured before cw without sensitive = %v, want en", ids)
	}

	if ids := list(service.NewsQuery{Reader: reader}); !sameIDs(ids, mineDE.ID()) {
		t.Fatalf("subscribed = %v, want mine", ids)
	}

	// An edit keeps the ID and the time it was published.
	edited, err := en.Edited(fedi.NoteDoc{ID: en.URI().String(), AttributedTo: featured.String(), Text: "edited", Public: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = news.Save(ctx, edited); err != nil {
		t.Fatal(err)
	}

	if p, err := news.Get(ctx, en.ID()); err != nil || p.Text() != "edited" || p.Updated().IsZero() || !p.Published().Equal(at) {
		t.Fatalf("Get after an edit = %+v, %v", p, err)
	}

	if err = news.Delete(ctx, en.URI()); err != nil {
		t.Fatal(err)
	}
	if _, err = news.Get(ctx, en.ID()); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Get of a deleted post = %v", err)
	}
	if _, err = covers.Get(ctx, en.ID(), video); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Get of a frame of a deleted post = %v", err)
	}

	if n, err := news.DeleteBefore(ctx, at.Add(time.Second+time.Millisecond)); err != nil || n < 1 {
		t.Fatalf("DeleteBefore = %d, %v", n, err)
	}

	if err = news.DeleteActor(ctx, featured); err != nil {
		t.Fatal(err)
	}
	if ids := list(service.NewsQuery{Featured: true, Languages: []string{"ru", "en"}}); len(ids) != 0 {
		t.Fatalf("featured after DeleteActor = %v", ids)
	}

	if err = subscriptions.RemoveActor(ctx, read); err != nil {
		t.Fatal(err)
	}
	if ok, err := subscriptions.Read(ctx, read); err != nil || ok {
		t.Fatalf("Read after RemoveActor = %v, %v", ok, err)
	}

	if err = following.Delete(ctx, featured); err != nil {
		t.Fatal(err)
	}
	if _, err = following.Get(ctx, featured); !errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Get after Delete = %v", err)
	}
}

func containsFollow(fs []*fedi.Following, actor fedi.URL) bool {
	for _, f := range fs {
		if f.Actor() == actor {
			return true
		}
	}

	return false
}

func sameIDs(got []uuid.UUID, want ...uuid.UUID) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
