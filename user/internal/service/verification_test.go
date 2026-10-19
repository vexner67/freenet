package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestVerificationService_Set(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	verification := service.NewVerificationService(w.users, w.users, admins)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	verified := func() bool {
		u, err := w.users.GetByID(ctx, ann)
		if err != nil {
			t.Fatal(err)
		}
		return u.IsVerified()
	}

	// Only the administrators give the checkmark.
	if err := verification.Set(ctx, ann, ann, true); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Set by ann: %v", err)
	}
	if err := verification.Set(ctx, admin, uuid.New(), true); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}

	if err := verification.Set(ctx, admin, ann, true); err != nil || !verified() {
		t.Fatalf("Set = %v, verified %v", err, verified())
	}

	// Saving the profile keeps the checkmark given meanwhile.
	if _, err := w.userSvc.UpdateProfile(ctx, ann, user.Profile{Name: "Анна", Nickname: "ann"}, ""); err != nil || !verified() {
		t.Fatalf("UpdateProfile = %v, verified %v", err, verified())
	}

	q, _ := user.NewSearchQuery("", user.SearchFilters{Verified: true})
	page, err := w.userSvc.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(page.Users) != 1 || page.Users[0].ID != ann || !page.Users[0].Verified {
		t.Fatalf("the verified pages: %+v, %v", page, err)
	}

	if err = verification.Set(ctx, admin, ann, false); err != nil || verified() {
		t.Fatalf("taking it: %v, verified %v", err, verified())
	}
}
