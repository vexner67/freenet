package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestEconomyRepository(t *testing.T) {
	_, pool := newRepo(t)
	economies := postgres.NewEconomyRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Without its row the economy is the default; the test leaves it so.
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM economy`); err != nil {
			t.Error(err)
		}
	})

	if e, err := economies.Economy(ctx); err != nil || e != user.DefaultEconomy {
		t.Fatalf("Economy = %+v, %v; want the default", e, err)
	}

	for _, want := range []user.Economy{
		{Bonus: 10_000, TransferFee: 2, StickerTax: 8, PrimeStickerTax: 4},
		{Bonus: user.MaxAmount, TransferFee: 0, StickerTax: 100, PrimeStickerTax: 0},
	} {
		if err := economies.SetEconomy(ctx, want); err != nil {
			t.Fatal(err)
		}
		if e, err := economies.Economy(ctx); err != nil || e != want {
			t.Fatalf("Economy = %+v, %v; want %+v", e, err, want)
		}
	}
}
