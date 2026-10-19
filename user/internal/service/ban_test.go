package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestBanService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	bans := service.NewBanService(w.users, w.users, admins)
	accounts := service.NewAccountService(w.users, nil, nil, time.Hour)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	banned := func() bool {
		state, err := accounts.State(ctx, ann)
		if err != nil {
			t.Fatal(err)
		}
		return state.Banned
	}

	tests := []struct {
		name          string
		admin, target uuid.UUID
		until         time.Time
		want          error
	}{
		{"by anyone", ann, admin, time.Time{}, user.ErrNotAdmin},
		{"an unknown page", admin, uuid.New(), time.Time{}, user.ErrNotFound},
		{"themselves", admin, admin, time.Time{}, user.ErrBanSelf},
		{"in the past", admin, ann, time.Now().Add(-time.Hour), user.ErrBanUntilPast},
	}
	for _, tt := range tests {
		if err := bans.Ban(ctx, tt.admin, tt.target, "spam", tt.until); !errors.Is(err, tt.want) {
			t.Fatalf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}

	if err := bans.Ban(ctx, admin, ann, "spam", time.Now().Add(time.Hour)); err != nil || !banned() {
		t.Fatalf("Ban = %v, banned %v", err, banned())
	}

	// Saving the profile keeps the block given meanwhile.
	if _, err := w.userSvc.UpdateProfile(ctx, ann, user.Profile{Name: "Анна", Nickname: "ann"}, ""); err != nil || !banned() {
		t.Fatalf("UpdateProfile = %v, banned %v", err, banned())
	}

	q, _ := user.NewSearchQuery("", user.SearchFilters{Banned: true})
	page, err := w.userSvc.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(page.Users) != 1 || page.Users[0].ID != ann || page.Users[0].Ban.Reason != "spam" {
		t.Fatalf("the blocked pages: %+v, %v", page, err)
	}

	if err = bans.Unban(ctx, ann, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Unban by ann: %v", err)
	}
	if err = bans.Unban(ctx, admin, ann); err != nil || banned() {
		t.Fatalf("Unban = %v, banned %v", err, banned())
	}

	// The administrators are not blocked: another one would lose the panel.
	both, _ := user.NewAdmins([]string{"admin@example.com", "ann@example.com"})
	if err = service.NewBanService(w.users, w.users, both).Ban(ctx, admin, ann, "spam", time.Time{}); !errors.Is(err, user.ErrBanAdmin) {
		t.Fatalf("an administrator: %v", err)
	}
}
