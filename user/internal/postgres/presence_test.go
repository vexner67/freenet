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

	if err := repo.MarkSeen(ctx, map[uuid.UUID]time.Time{bob.ID(): now}); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	// Told late: the later visit stays.
	if err := repo.MarkSeen(ctx, map[uuid.UUID]time.Time{bob.ID(): now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	_ = repo.MarkSeen(ctx, map[uuid.UUID]time.Time{cat.ID(): now.Add(-time.Hour)})

	// Counted with everyone else in the table: at least these, of whom Bob
	// alone came in the last minutes.
	recent, err := repo.Count(ctx, now.Add(-time.Minute))
	if err != nil || recent.Users < 2 || recent.Online < 1 {
		t.Fatalf("Count = %+v, %v", recent, err)
	}
	// The same people, fewer of them recent: other tests share the table,
	// so only the order is sure.
	if later, err := repo.Count(ctx, now.Add(time.Hour)); err != nil || later.Online > recent.Online || later.Users < 2 {
		t.Fatalf("Count since later = %+v, %v", later, err)
	}
	// Many visits go at once; the one of nobody is left out.
	if err = repo.MarkSeen(ctx, map[uuid.UUID]time.Time{dan.ID(): now, uuid.New(): now, cat.ID(): now.Add(-2 * time.Hour)}); err != nil {
		t.Fatalf("MarkSeen, many: %v", err)
	}
	if err = repo.MarkSeen(ctx, nil); err != nil {
		t.Fatalf("MarkSeen, none: %v", err)
	}
	if seen, _ := repo.GetByID(ctx, cat.ID()); !seen.LastSeen().Equal(now.Add(-time.Hour)) {
		t.Fatalf("cat, told late among many: %v", seen.LastSeen())
	}

	got, err := repo.GetByID(ctx, bob.ID())
	if err != nil || !got.LastSeen().Equal(now) {
		t.Fatalf("last seen %v, %v", got.LastSeen(), err)
	}

	// Saving the page keeps the visit.
	if err = got.UpdateProfile(user.Profile{Name: "Bob", Nickname: user.Nickname(unique("bob"))}); err != nil {
		t.Fatal(err)
	}
	_ = repo.MarkSeen(ctx, map[uuid.UUID]time.Time{bob.ID(): now.Add(time.Second)})
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

	// A backdrop changes one side and tells what it replaced; Update keeps it.
	photo := uuid.New()
	if prev, err := repo.SetBackdrop(ctx, cat.ID(), user.BackdropLeft, photo); err != nil || prev != uuid.Nil {
		t.Fatalf("SetBackdrop = %v, %v", prev, err)
	}
	if prev, err := repo.SetBackdrop(ctx, cat.ID(), user.BackdropRight, photo); err != nil || prev != uuid.Nil {
		t.Fatalf("the right side = %v, %v", prev, err)
	}
	c, _ := repo.GetByID(ctx, cat.ID())
	if prev, err := repo.SetBackdrop(ctx, cat.ID(), user.BackdropLeft, uuid.Nil); err != nil || prev != photo {
		t.Fatalf("clearing = %v, %v", prev, err)
	}
	if err = repo.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	if c, _ = repo.GetByID(ctx, cat.ID()); c.Backdrop() != (user.Backdrop{Right: photo}) {
		t.Fatalf("backdrop %+v", c.Backdrop())
	}
	if _, err = repo.SetBackdrop(ctx, uuid.New(), user.BackdropLeft, photo); err != user.ErrNotFound {
		t.Fatalf("nobody: %v", err)
	}

	// Whom ann and bob both follow: cat; bob also follows ann.
	if err = follows.Follow(ctx, bob.ID(), cat.ID()); err != nil {
		t.Fatal(err)
	}
	_ = follows.Follow(ctx, bob.ID(), ann.ID())
	common, err := follows.ListCommonFollowing(ctx, ann.ID(), bob.ID(), uuid.Nil, 10)
	if err != nil || len(common) != 1 || common[0].User.ID() != cat.ID() {
		t.Fatalf("ListCommonFollowing = %d, %v", len(common), err)
	}
	if n, err := follows.CountCommonFollowing(ctx, ann.ID(), bob.ID()); err != nil || n != 1 {
		t.Fatalf("CountCommonFollowing = %d, %v", n, err)
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
