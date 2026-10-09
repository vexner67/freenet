package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPresence(t *testing.T) {
	repo, pool := newRepo(t)
	follows := postgres.NewFollowRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("pr") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u
	}

	// Postgres keeps microseconds.
	now := time.Now().UTC().Truncate(time.Microsecond)
	ann, bob, cat, dan := newUser(), newUser(), newUser(), newUser()
	for _, followee := range []uuid.UUID{bob.ID(), cat.ID(), dan.ID()} {
		if err := follows.Follow(ctx, ann.ID(), followee); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.MarkSeen(ctx, bob.ID(), now); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	// Told late: the later visit stays.
	if err := repo.MarkSeen(ctx, bob.ID(), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	_ = repo.MarkSeen(ctx, cat.ID(), now.Add(-time.Hour))
	_ = repo.MarkSeen(ctx, dan.ID(), now)

	got, err := repo.GetByID(ctx, bob.ID())
	if err != nil || !got.LastSeen().Equal(now) {
		t.Fatalf("last seen %v, %v", got.LastSeen(), err)
	}

	// Saving the page keeps the visit.
	if err = got.UpdateProfile(user.Profile{Name: "Bob", Nickname: user.Nickname(unique("bob"))}); err != nil {
		t.Fatal(err)
	}
	_ = repo.MarkSeen(ctx, bob.ID(), now.Add(time.Second))
	if err = repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.GetByID(ctx, bob.ID()); !got.LastSeen().Equal(now.Add(time.Second)) {
		t.Fatalf("after Update: %v", got.LastSeen())
	}

	// Dan deleted the page: not online for anyone.
	d, _ := repo.GetByID(ctx, dan.ID())
	_ = d.Deactivate("", now, time.Hour)
	if err = repo.Update(ctx, d); err != nil {
		t.Fatal(err)
	}

	since := now.Add(-user.OnlineFor)
	online, err := follows.ListOnlineFollowing(ctx, ann.ID(), since, uuid.Nil, 10)
	if err != nil || len(online) != 1 || online[0].User.ID() != bob.ID() {
		t.Fatalf("ListOnlineFollowing = %d, %v", len(online), err)
	}
	if n, err := follows.CountOnlineFollowing(ctx, ann.ID(), since); err != nil || n != 1 {
		t.Fatalf("CountOnlineFollowing = %d, %v", n, err)
	}
	if online, _ = follows.ListOnlineFollowing(ctx, ann.ID(), since, online[0].FollowID, 10); len(online) != 0 {
		t.Fatalf("after the cursor: %d", len(online))
	}
}
