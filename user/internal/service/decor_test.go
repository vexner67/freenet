package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestDecorService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	decors := service.NewDecorService(w.users, inmem.NewDecorRepository(), admins)

	if decor, _, err := decors.Decor(ctx); err != nil || decor != user.DecorAuto {
		t.Fatalf("Decor = %q, %v; want auto", decor, err)
	}

	// Only the administrators dress the site.
	if err := decors.SetDecor(ctx, ann, user.DecorOn); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetDecor by ann: %v", err)
	}

	for _, tt := range []struct {
		decor   user.Decor
		festive bool
	}{{user.DecorOn, true}, {user.DecorOff, false}} {
		if err := decors.SetDecor(ctx, admin, tt.decor); err != nil {
			t.Fatal(err)
		}
		if decor, festive, err := decors.Decor(ctx); err != nil || decor != tt.decor || festive != tt.festive {
			t.Fatalf("Decor = %q, %v, %v; want %q, %v", decor, festive, err, tt.decor, tt.festive)
		}
	}
}
