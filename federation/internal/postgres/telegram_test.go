package postgres_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/postgres"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func TestFollowingChannels(t *testing.T) {
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
	now := time.Now().UTC()

	// Of their own, so that runs do not meet.
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	channel := fedi.ChannelURL("c" + suffix)
	account := fedi.URL("https://m.example/users/" + suffix)

	for _, actor := range []fedi.URL{channel, account} {
		f, err := fedi.NewFollowing(actor, false, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = following.Save(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	channels, err := following.Channels(ctx)
	if err != nil || !slices.Contains(channels, channel) || slices.Contains(channels, account) {
		t.Fatalf("Channels = %v, %v; want %s and not %s", channels, err, channel, account)
	}
}

func TestChannelPostsAndLikes(t *testing.T) {
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

	posts := postgres.NewRemotePostRepository(pool)
	likes := postgres.NewLikeRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	channel := fedi.ChannelURL("c" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20])
	note := func(n string) fedi.NoteDoc {
		id := channel.String() + "/" + n
		return fedi.NoteDoc{ID: id, AttributedTo: channel.String(), URL: id, Text: "news " + n, Public: true, Published: now.Add(-time.Hour)}
	}

	withCard := note("1")
	withCard.Link = fedi.LinkDoc{URL: "https://meduza.io/news/1", Site: "Meduza", Title: "Новость", Image: "https://cdn4.telesco.pe/file/card.jpg"}

	var saved []*fedi.RemotePost
	for _, d := range []fedi.NoteDoc{withCard, note("2")} {
		p, err := fedi.NewRemotePost(d, channel, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
		saved = append(saved, p)
	}

	got, err := posts.Get(ctx, saved[0].ID())
	if err != nil || got.Link() != saved[0].Link() {
		t.Fatalf("Get = %+v, %v; want the card %+v", got, err, saved[0].Link())
	}
	if got, err = posts.Get(ctx, saved[1].ID()); err != nil || got.Link() != (fedi.Link{}) {
		t.Fatalf("Get of a post with no card = %+v, %v", got, err)
	}

	uris, err := posts.URIsOf(ctx, channel)
	slices.Sort(uris)
	if err != nil || !slices.Equal(uris, []fedi.URL{saved[0].URI(), saved[1].URI()}) {
		t.Fatalf("URIsOf = %v, %v", uris, err)
	}

	reader, other := uuid.New(), uuid.New()
	for _, user := range []uuid.UUID{reader, other, reader} {
		if err = likes.Add(ctx, saved[0].ID(), service.Liker{ID: uuid.Must(uuid.NewV7()), UserID: user}, now); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	of, err := likes.Of(ctx, []uuid.UUID{saved[0].ID(), saved[1].ID()}, reader)
	if err != nil || len(of) != 1 || of[saved[0].ID()] != (service.Likes{Count: 2, Liked: true}) {
		t.Fatalf("Of = %+v, %v", of, err)
	}

	likers, err := likes.Likers(ctx, saved[0].ID(), uuid.Nil, 1)
	if err != nil || len(likers) != 1 || likers[0].UserID != other {
		t.Fatalf("Likers = %+v, %v; want the latest first", likers, err)
	}
	if likers, err = likes.Likers(ctx, saved[0].ID(), likers[0].ID, 10); err != nil || len(likers) != 1 || likers[0].UserID != reader {
		t.Fatalf("Likers after the cursor = %+v, %v", likers, err)
	}

	if err = likes.Remove(ctx, saved[0].ID(), reader); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err = posts.Delete(ctx, saved[0].URI()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if of, err = likes.Of(ctx, []uuid.UUID{saved[0].ID()}, reader); err != nil || len(of) != 0 {
		t.Fatalf("Of a post deleted = %+v, %v; the likes go with it", of, err)
	}

	if err = posts.DeleteActor(ctx, channel); err != nil {
		t.Fatal(err)
	}
}
