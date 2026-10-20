package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestDecorRepository(t *testing.T) {
	_, pool := newRepo(t)
	decors := postgres.NewDecorRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts it by the calendar; the test leaves it there.
	t.Cleanup(func() {
		if err := decors.SetDecor(context.Background(), user.DecorAuto); err != nil {
			t.Error(err)
		}
	})

	if decor, err := decors.Decor(ctx); err != nil || decor != user.DecorAuto {
		t.Fatalf("Decor = %q, %v; want auto", decor, err)
	}

	for _, want := range []user.Decor{user.DecorOn, user.DecorOff} {
		if err := decors.SetDecor(ctx, want); err != nil {
			t.Fatal(err)
		}
		if decor, err := decors.Decor(ctx); err != nil || decor != want {
			t.Fatalf("Decor = %q, %v; want %q", decor, err, want)
		}
	}
}
