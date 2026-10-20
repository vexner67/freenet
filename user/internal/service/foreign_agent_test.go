package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestForeignAgentService_Set(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	agents := service.NewForeignAgentService(w.users, w.users, admins)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	marked := func() bool {
		u, err := w.users.GetByID(ctx, ann)
		if err != nil {
			t.Fatal(err)
		}
		return u.IsForeignAgent()
	}

	// Only the administrators put the plaque.
	if err := agents.Set(ctx, ann, ann, true); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Set by ann: %v", err)
	}
	if err := agents.Set(ctx, admin, uuid.New(), true); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}

	if err := agents.Set(ctx, admin, ann, true); err != nil || !marked() {
		t.Fatalf("Set = %v, marked %v", err, marked())
	}

	// Saving the profile keeps the plaque put meanwhile.
	if _, err := w.userSvc.UpdateProfile(ctx, ann, user.Profile{Name: "Анна", Nickname: "ann"}, ""); err != nil || !marked() {
		t.Fatalf("UpdateProfile = %v, marked %v", err, marked())
	}

	if err := agents.Set(ctx, admin, ann, false); err != nil || marked() {
		t.Fatalf("taking it: %v, marked %v", err, marked())
	}
}
