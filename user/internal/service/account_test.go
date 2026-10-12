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

// fakeReactions records whose likes and votes were set aside and put back.
type fakeReactions struct {
	held, released []uuid.UUID
	err            error
}

func (f *fakeReactions) UserDeactivated(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.held = append(f.held, id)
	return nil
}

func (f *fakeReactions) UserReactivated(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.released = append(f.released, id)
	return nil
}

func TestAccountService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	userSvc := service.NewUserService(users)
	reactions := &fakeReactions{}
	accounts := service.NewAccountService(users, inmem.TxManager{}, reactions, time.Hour)
	followSvc := service.NewFollowService(service.FollowDeps{
		Users: users, Follows: follows, Lists: follows, Blocks: inmem.NewBlockRepository(follows),
		Tx: inmem.TxManager{}, Notifier: &fakeNotifier{},
	})

	register := func(email, nickname string) uuid.UUID {
		t.Helper()

		id, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, email))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = userSvc.UpdateProfile(ctx, id, user.Profile{Name: "Анна", Nickname: mustNickname(t, nickname)}, ""); err != nil {
			t.Fatal(err)
		}

		return id
	}

	ann, bob := register("ann@example.com", "ann"), register("bob@example.com", "bob")

	if _, err := accounts.Reactivate(ctx, ann); !errors.Is(err, user.ErrNotDeactivated) {
		t.Fatalf("Reactivate an active page: %v", err)
	}

	u, err := accounts.Deactivate(ctx, ann, "надоело")
	if err != nil {
		t.Fatal(err)
	}
	if d := u.Deactivation(); !u.IsDeactivated() || d.PurgeAt.Sub(d.At) != time.Hour || d.Reason != "надоело" {
		t.Fatalf("deactivation = %+v", d)
	}
	if deactivated, err := accounts.IsDeactivated(ctx, ann); err != nil || !deactivated {
		t.Fatalf("IsDeactivated = %v, %v", deactivated, err)
	}
	if _, err = accounts.Deactivate(ctx, ann, ""); !errors.Is(err, user.ErrDeactivated) {
		t.Fatalf("Deactivate twice: %v", err)
	}
	// Her likes and votes are set aside once, with the deletion.
	if len(reactions.held) != 1 || reactions.held[0] != ann || len(reactions.released) != 0 {
		t.Fatalf("held %v, released %v; want ann held once", reactions.held, reactions.released)
	}

	// Hidden: nobody finds or follows it; its profile has the name only.
	q, err := user.NewSearchQuery("анна", user.SearchFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if page, err := userSvc.Search(ctx, q, uuid.Nil, 10); err != nil || len(page.Users) != 1 || page.Users[0].ID != bob {
		t.Fatalf("search finds %+v, %v; want bob only", page.Users, err)
	}
	if err = followSvc.Follow(ctx, bob, ann); !errors.Is(err, user.ErrDeactivated) {
		t.Fatalf("follow a deleted page: %v", err)
	}
	if p, err := service.NewPermissionService(users, follows, inmem.NewBlockRepository(follows)).ProfileFor(ctx, bob, "ann"); err != nil || !p.Deactivated || p.Name != "Анна" {
		t.Fatalf("public profile = %+v, %v", p, err)
	}

	if u, err = accounts.Reactivate(ctx, ann); err != nil || u.IsDeactivated() {
		t.Fatalf("Reactivate = %v", err)
	}
	if len(reactions.held) != 1 || len(reactions.released) != 1 || reactions.released[0] != ann {
		t.Fatalf("held %v, released %v; want ann released once", reactions.held, reactions.released)
	}
	if err = followSvc.Follow(ctx, bob, ann); err != nil {
		t.Fatalf("follow the restored page: %v", err)
	}

	// What cannot be recorded fails the deletion.
	reactions.err = errors.New("outbox down")
	if _, err = accounts.Deactivate(ctx, ann, ""); err == nil {
		t.Fatal("Deactivate with a failing outbox: no error")
	}
	reactions.err = nil

	if _, err = accounts.Deactivate(ctx, uuid.New(), ""); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Deactivate nobody: %v", err)
	}
}
