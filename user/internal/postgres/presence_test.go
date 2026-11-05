package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
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

	// Ann follows bob, cat and dan; all three follow back, and bob and cat
	// are friends too: cat is a friend of both ann and bob.
	for _, f := range [][2]uuid.UUID{{bob.ID(), ann.ID()}, {cat.ID(), ann.ID()}, {dan.ID(), ann.ID()}, {bob.ID(), cat.ID()}, {cat.ID(), bob.ID()}} {
		if err = follows.Follow(ctx, f[0], f[1]); err != nil {
			t.Fatal(err)
		}
	}

	common := service.FriendQuery{UserID: ann.ID(), ViewerID: bob.ID(), Section: user.FriendsCommon}
	if found, err := follows.ListFriends(ctx, common, uuid.Nil, 10); err != nil || len(found) != 1 || found[0].User.ID() != cat.ID() {
		t.Fatalf("common friends = %d, %v", len(found), err)
	}
	if n, err := follows.CountFriends(ctx, common); err != nil || n != 1 {
		t.Fatalf("common friends counted = %d, %v", n, err)
	}

	online := service.FriendQuery{UserID: ann.ID(), Section: user.FriendsOnline, Since: now.Add(-user.OnlineFor)}
	found, err := follows.ListFriends(ctx, online, uuid.Nil, 10)
	if err != nil || len(found) != 1 || found[0].User.ID() != bob.ID() {
		t.Fatalf("online friends = %d, %v", len(found), err)
	}
	if n, err := follows.CountFriends(ctx, online); err != nil || n != 1 {
		t.Fatalf("online friends counted = %d, %v", n, err)
	}
	if found, _ = follows.ListFriends(ctx, online, found[0].FollowID, 10); len(found) != 0 {
		t.Fatalf("after the cursor: %d", len(found))
	}
}
