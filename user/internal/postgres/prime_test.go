package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPrimeRepository(t *testing.T) {
	repo, pool := newRepo(t)
	primes := postgres.NewPrimeRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts with the defaults; the test leaves them there.
	t.Cleanup(func() {
		if err := primes.SetPrices(context.Background(), user.DefaultPrimePrices); err != nil {
			t.Error(err)
		}
	})

	if prices, err := primes.Prices(ctx); err != nil || prices != user.DefaultPrimePrices {
		t.Fatalf("Prices = %+v, %v", prices, err)
	}
	cheap, _ := user.NewPrimePrices(1, 2, 3)
	if err := primes.SetPrices(ctx, cheap); err != nil {
		t.Fatal(err)
	}
	if prices, err := primes.Prices(ctx); err != nil || prices != cheap {
		t.Fatalf("Prices after = %+v, %v", prices, err)
	}

	u, err := user.New(user.Email(unique("prime") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)

	until, err := primes.Extend(ctx, u.ID(), user.PrimeMonth, now)
	if err != nil || !until.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("Extend = %v, %v", until, err)
	}
	// A lasting subscription grows from its end.
	if until, err = primes.Extend(ctx, u.ID(), user.PrimeYear, now); err != nil || !until.Equal(now.AddDate(0, 0, 395)) {
		t.Fatalf("Extend again = %v, %v", until, err)
	}
	if _, err = primes.Extend(ctx, uuid.New(), user.PrimeMonth, now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Extend nobody: %v", err)
	}

	if err = primes.SetInvisible(ctx, u.ID(), true); err != nil {
		t.Fatal(err)
	}
	if err = primes.SetInvisible(ctx, uuid.New(), true); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("SetInvisible nobody: %v", err)
	}

	style, err := user.NewPrimeStyle("sapphire", "silver", "sparkles", "sleepy")
	if err != nil {
		t.Fatal(err)
	}
	if err = primes.SetStyle(ctx, u.ID(), style); err != nil {
		t.Fatal(err)
	}
	if err = primes.SetStyle(ctx, uuid.New(), style); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("SetStyle nobody: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil || !got.Prime().Until.Equal(until) || !got.Prime().Invisible || !got.IsPrimeAt(now) {
		t.Fatalf("read back = %+v, %v", got.Prime(), err)
	}
	if got.PrimeStyle() != style {
		t.Fatalf("style read back = %+v", got.PrimeStyle())
	}

	// The gold theme of Prime is stored.
	gold, err := user.NewInterface(user.InterfaceForm{AvatarStyle: "square", Theme: "gold", NSFW: "blur", PostView: "old", MainPage: "page"})
	if err != nil {
		t.Fatal(err)
	}
	if err = got.UpdateInterface(gold, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || got.Interface().Theme != user.ThemeGold {
		t.Fatalf("gold theme = %v, %v", got.Interface().Theme, err)
	}
}
