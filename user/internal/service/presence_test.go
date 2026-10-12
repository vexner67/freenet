package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPresenceService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	blocks := inmem.NewBlockRepository(follows)

	userSvc := service.NewUserService(users)
	presence := service.NewPresenceService(users)
	followSvc := service.NewFollowService(service.FollowDeps{
		Users: users, Follows: follows, Lists: follows, Blocks: blocks, Tx: inmem.TxManager{}, Notifier: &fakeNotifier{},
	})

	var ids []uuid.UUID
	for _, nick := range []string{"ann", "bob", "cat"} {
		id, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, nick+"@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = userSvc.UpdateProfile(ctx, id, user.Profile{Name: "N", Nickname: user.Nickname(nick)}, ""); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	ann, bob, cat := ids[0], ids[1], ids[2]

	for _, followee := range []uuid.UUID{bob, cat} {
		if err := followSvc.Follow(ctx, ann, followee); err != nil {
			t.Fatal(err)
		}
	}

	if err := presence.MarkOnline(ctx, bob); err != nil {
		t.Fatalf("MarkOnline: %v", err)
	}

	// The visit waits to be written with the others.
	if stats, err := presence.Statistics(ctx); err != nil || stats.Online != 0 {
		t.Fatalf("before the flush: %+v, %v", stats, err)
	}
	if err := presence.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	page, err := followSvc.ListOnlineFollowing(ctx, ann, uuid.Nil, 0)
	if err != nil || page.Total != 1 || len(page.Users) != 1 || page.Users[0].ID != bob {
		t.Fatalf("ListOnlineFollowing = %+v, %v", page, err)
	}

	// The site counts its people and who of them is on it.
	if stats, err := presence.Statistics(ctx); err != nil || stats != (service.Statistics{Users: 3, Online: 1}) {
		t.Fatalf("Statistics = %+v, %v", stats, err)
	}

	// The page of someone online says so; of someone never seen, nothing.
	perms := service.NewPermissionService(users, follows, blocks)
	if p, err := perms.ProfileFor(ctx, uuid.Nil, "bob"); err != nil || !p.Online || p.LastSeen.IsZero() {
		t.Fatalf("bob: %+v, %v", p, err)
	}
	if p, err := perms.ProfileFor(ctx, uuid.Nil, "cat"); err != nil || p.Online || !p.LastSeen.IsZero() {
		t.Fatalf("cat: %+v, %v", p, err)
	}
}

// failingPresence refuses visits until told otherwise and keeps what it
// was given.
type failingPresence struct {
	service.PresenceRepository
	err    error
	visits []map[uuid.UUID]time.Time
}

func (f *failingPresence) MarkSeen(_ context.Context, visits map[uuid.UUID]time.Time) error {
	if f.err != nil {
		return f.err
	}

	f.visits = append(f.visits, visits)

	return nil
}

// Visits that could not be written wait for the next flush; nothing to
// write asks nothing of the storage.
func TestPresenceService_Flush(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := &failingPresence{err: errors.New("down")}
	presence := service.NewPresenceService(repo)
	ann, bob := uuid.New(), uuid.New()

	if err := presence.Flush(ctx); err != nil {
		t.Fatalf("Flush of nothing: %v", err)
	}

	for _, id := range []uuid.UUID{ann, ann, bob} {
		if err := presence.MarkOnline(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := presence.Flush(ctx); err == nil {
		t.Fatal("Flush to a storage that is down: no error")
	}

	repo.err = nil
	if err := presence.MarkOnline(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if err := presence.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(repo.visits) != 1 || len(repo.visits[0]) != 2 || repo.visits[0][ann].IsZero() || repo.visits[0][bob].IsZero() {
		t.Fatalf("written %v, want one visit of each", repo.visits)
	}

	if err := presence.Flush(ctx); err != nil || len(repo.visits) != 1 {
		t.Fatalf("second Flush: %v, written %d times", err, len(repo.visits))
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := presence.MarkOnline(cancelled, ann); err == nil {
		t.Fatal("MarkOnline with a cancelled context: no error")
	}
}

// Run writes what waits when it is stopped.
func TestPresenceService_Run(t *testing.T) {
	t.Parallel()

	repo := &failingPresence{}
	presence := service.NewPresenceService(repo)
	ann := uuid.New()

	if err := presence.MarkOnline(context.Background(), ann); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	presence.Run(ctx, time.Hour, func(err error) { t.Errorf("flush failed: %v", err) })

	if len(repo.visits) != 1 || repo.visits[0][ann].IsZero() {
		t.Fatalf("written %v, want the visit of ann", repo.visits)
	}
}
