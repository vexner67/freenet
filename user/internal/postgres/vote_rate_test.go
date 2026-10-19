package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestVoteRateRepository(t *testing.T) {
	_, pool := newRepo(t)
	rates := postgres.NewVoteRateRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts it at the default; the test leaves it there.
	t.Cleanup(func() {
		if err := rates.SetRate(context.Background(), user.DefaultVoteRate); err != nil {
			t.Error(err)
		}
	})

	if rate, err := rates.Rate(ctx); err != nil || rate != user.DefaultVoteRate {
		t.Fatalf("Rate = %d, %v; want the default", rate, err)
	}

	for _, kopecks := range []int64{1250, 99} {
		if err := rates.SetRate(ctx, user.VoteRate(kopecks)); err != nil {
			t.Fatal(err)
		}
		if rate, err := rates.Rate(ctx); err != nil || rate.Kopecks() != kopecks {
			t.Fatalf("Rate = %d, %v; want %d", rate, err, kopecks)
		}
	}
}
