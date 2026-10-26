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

func TestTitleService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	if _, err := w.userSvc.UpdateProfile(ctx, ann, profile(t, "ann"), ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	titles := service.NewTitleService(w.users, inmem.NewTitleRepository(w.users), admins)

	// Only the administrators give titles, to pages that are.
	if _, err = titles.Grant(ctx, ann, "ann", "Король шахты", user.TitleCrown, now); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Grant by ann: %v", err)
	}
	if _, err = titles.Grant(ctx, admin, "nobody", "Король шахты", user.TitleCrown, now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Grant to nobody: %v", err)
	}

	// The same title may be given again and again.
	first, err := titles.Grant(ctx, admin, "ann", "Король шахты", user.TitleCrown, now)
	if err != nil || first.UserID() != ann || first.Name() != "Король шахты" || first.Icon() != user.TitleCrown {
		t.Fatalf("Grant = %+v, %v", first, err)
	}
	second, err := titles.Grant(ctx, admin, "ann", "Король шахты", user.TitleCrown, now)
	if err != nil {
		t.Fatal(err)
	}
	cup, err := titles.Grant(ctx, admin, "ann", "Игрок недели", user.TitleCup, now)
	if err != nil {
		t.Fatal(err)
	}

	got, err := titles.UserTitles(ctx, "ann", now)
	if err != nil || len(got) != 3 || got[0].ID() != cup.ID() || got[1].ID() != second.ID() || got[2].ID() != first.ID() {
		t.Fatalf("UserTitles = %+v, %v", got, err)
	}

	if err = titles.Revoke(ctx, ann, first.ID()); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Revoke by ann: %v", err)
	}
	if err = titles.Revoke(ctx, admin, first.ID()); err != nil {
		t.Fatal(err)
	}
	if err = titles.Revoke(ctx, admin, first.ID()); !errors.Is(err, user.ErrTitleNotFound) {
		t.Fatalf("Revoke twice: %v", err)
	}
	if err = titles.Revoke(ctx, admin, uuid.New()); !errors.Is(err, user.ErrTitleNotFound) {
		t.Fatalf("Revoke of none: %v", err)
	}
	if got, err = titles.UserTitles(ctx, "ann", now); err != nil || len(got) != 2 {
		t.Fatalf("UserTitles after Revoke = %+v, %v", got, err)
	}

	// A deleted page shows none.
	if _, err = w.accounts.Deactivate(ctx, ann, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = titles.UserTitles(ctx, "ann", now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("UserTitles of a deleted page: %v", err)
	}
	if _, err = titles.UserTitles(ctx, "nobody", now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("UserTitles of nobody: %v", err)
	}
}
