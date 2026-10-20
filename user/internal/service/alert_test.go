package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestAlertService_Set(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	alerts := service.NewAlertService(w.users, w.users, admins)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	const label = user.Alert("Страница под наблюдением")
	alertOf := func() user.Alert {
		u, err := w.users.GetByID(ctx, ann)
		if err != nil {
			t.Fatal(err)
		}
		return u.Alert()
	}

	// Only the administrators put the label.
	if err := alerts.Set(ctx, ann, ann, label); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Set by ann: %v", err)
	}
	if err := alerts.Set(ctx, admin, uuid.New(), label); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}

	if err := alerts.Set(ctx, admin, ann, label); err != nil || alertOf() != label {
		t.Fatalf("Set = %v, label %q", err, alertOf())
	}

	// Saving the profile keeps the label put meanwhile.
	if _, err := w.userSvc.UpdateProfile(ctx, ann, user.Profile{Name: "Анна", Nickname: "ann"}, ""); err != nil || alertOf() != label {
		t.Fatalf("UpdateProfile = %v, label %q", err, alertOf())
	}

	if err := alerts.Set(ctx, admin, ann, ""); err != nil || alertOf() != "" {
		t.Fatalf("taking it: %v, label %q", err, alertOf())
	}
}
