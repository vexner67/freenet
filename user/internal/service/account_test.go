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

func TestAccountService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	userSvc := service.NewUserService(users)
	accounts := service.NewAccountService(users, time.Hour)
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
	if err = followSvc.Follow(ctx, bob, ann); err != nil {
		t.Fatalf("follow the restored page: %v", err)
	}

	if _, err = accounts.Deactivate(ctx, uuid.New(), ""); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Deactivate nobody: %v", err)
	}
}
