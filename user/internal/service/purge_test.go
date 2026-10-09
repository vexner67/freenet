package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// fakeEraser records whom the other services are told to erase, and fails
// with err when set.
type fakeEraser struct {
	erased []uuid.UUID
	err    error
}

func (f *fakeEraser) UserErased(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.erased = append(f.erased, id)
	return nil
}

func TestPurgeService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	userSvc := service.NewUserService(users)
	// A page deleted now is due at once. Not a nanosecond: the clock may
	// not have moved by the purge.
	accounts := service.NewAccountService(users, 0)
	followSvc := service.NewFollowService(service.FollowDeps{
		Users: users, Follows: follows, Lists: follows, Blocks: inmem.NewBlockRepository(follows),
		Tx: inmem.TxManager{}, Notifier: &fakeNotifier{},
	})
	eraser := &fakeEraser{}
	purge := service.NewPurgeService(users, inmem.TxManager{}, eraser)

	register := func(email, nickname string) uuid.UUID {
		t.Helper()

		id, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, email))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = userSvc.UpdateProfile(ctx, id, user.Profile{Name: "Имя", Nickname: mustNickname(t, nickname)}, ""); err != nil {
			t.Fatal(err)
		}
		return id
	}

	ann, bob, cat := register("ann@example.com", "ann"), register("bob@example.com", "bob"), register("cat@example.com", "cat")
	if err := followSvc.Follow(ctx, bob, ann); err != nil {
		t.Fatal(err)
	}

	if n, err := purge.PurgeDue(ctx); err != nil || n != 0 {
		t.Fatalf("nothing deleted yet: PurgeDue = %d, %v", n, err)
	}

	for _, id := range []uuid.UUID{ann, cat} {
		if _, err := accounts.Deactivate(ctx, id, ""); err != nil {
			t.Fatal(err)
		}
	}

	n, err := purge.PurgeDue(ctx)
	if err != nil || n != 2 {
		t.Fatalf("PurgeDue = %d, %v; want ann and cat", n, err)
	}
	if !slices.Contains(eraser.erased, ann) || !slices.Contains(eraser.erased, cat) || len(eraser.erased) != 2 {
		t.Fatalf("erased = %v", eraser.erased)
	}

	if _, err = userSvc.GetByID(ctx, ann); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("ann is still there: %v", err)
	}
	if stats, err := followSvc.Stats(ctx, bob, uuid.Nil); err != nil || stats.Following != 0 {
		t.Fatalf("bob still follows ann: %+v, %v", stats, err)
	}

	// The email can sign up again, as a new user.
	if again, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, "ann@example.com")); err != nil || again == ann {
		t.Fatalf("signing up again = %v, %v", again, err)
	}

	// The erasure elsewhere is recorded with the deletion: if it cannot
	// be, the purge fails (and PostgreSQL rolls the deletion back).
	if _, err = accounts.Deactivate(ctx, bob, ""); err != nil {
		t.Fatal(err)
	}
	eraser.err = errors.New("outbox is down")
	if _, err = purge.PurgeDue(ctx); !errors.Is(err, eraser.err) {
		t.Fatalf("PurgeDue with a broken outbox: %v, want its error", err)
	}
}
