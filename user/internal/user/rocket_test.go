package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewCasinoRocket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		least, most int64
		ret, top    int
		want        error
	}{
		{name: "fine", least: 1, most: 1000, ret: 9500, top: 100_000},
		{name: "the most", least: 1, most: user.MaxBet, ret: user.MaxRocketReturn, top: user.MaxRocketTop},
		{name: "free", least: 0, most: 10, ret: 9500, top: 1000, want: user.ErrRocketLimits},
		{name: "too big", least: 1, most: user.MaxBet + 1, ret: 9500, top: 1000, want: user.ErrRocketLimits},
		{name: "upside down", least: 10, most: 5, ret: 9500, top: 1000, want: user.ErrRocketLimits},
		{name: "no return", least: 1, most: 10, ret: 0, top: 1000, want: user.ErrRocketReturn},
		{name: "above all", least: 1, most: 10, ret: user.MaxRocketReturn + 1, top: 1000, want: user.ErrRocketReturn},
		{name: "low top", least: 1, most: 10, ret: 9500, top: user.MinRocketTop - 1, want: user.ErrRocketTop},
		{name: "high top", least: 1, most: 10, ret: 9500, top: user.MaxRocketTop + 1, want: user.ErrRocketTop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := user.NewCasinoRocket(tt.least, tt.most, tt.ret, tt.top)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewCasinoRocket = %v, want %v", err, tt.want)
			}
			if err == nil && (r.Least() != tt.least || r.Most() != tt.most || r.Return() != tt.ret || r.Top() != tt.top) {
				t.Fatalf("rocket = %+v", r)
			}
		})
	}
}

func TestCasinoRocket_Crash(t *testing.T) {
	t.Parallel()

	r, err := user.NewCasinoRocket(1, 100, 9500, 100_000)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		roll int64
		want int
	}{
		// Return / 1 is below ×1: the rocket explodes at the start.
		{name: "the start", roll: 0, want: 100},
		{name: "×1.9", roll: user.RocketRolls / 2, want: 190},
		{name: "×9.49", roll: user.RocketRolls / 10 * 9, want: 949},
		{name: "the top", roll: user.RocketRolls - 1, want: 100_000},
	}

	for _, tt := range tests {
		if got := r.Crash(tt.roll); got != tt.want {
			t.Errorf("%s: Crash(%d) = %d, want %d", tt.name, tt.roll, got, tt.want)
		}
	}
}

func TestRocketMultiplier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		elapsed time.Duration
		want    int
	}{
		{elapsed: -time.Second, want: 100},
		{elapsed: 0, want: 100},
		{elapsed: time.Second, want: 112},
		{elapsed: 5776 * time.Millisecond, want: 199},
		{elapsed: 5777 * time.Millisecond, want: 200},
		{elapsed: time.Hour, want: user.MaxRocketTop},
	}

	for _, tt := range tests {
		if got := user.RocketMultiplier(tt.elapsed); got != tt.want {
			t.Errorf("RocketMultiplier(%v) = %d, want %d", tt.elapsed, got, tt.want)
		}
	}
}

func TestRocketRound(t *testing.T) {
	t.Parallel()

	r, err := user.NewCasinoRocket(10, 1000, 9500, 100_000)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	id, ann := uuid.New(), uuid.New()

	if _, err := r.Launch(id, ann, 9, 0, 300, now); !errors.Is(err, user.ErrRocketAmount) {
		t.Fatalf("Launch below the least: %v", err)
	}
	if _, err := r.Launch(id, ann, 1001, 0, 300, now); !errors.Is(err, user.ErrRocketAmount) {
		t.Fatalf("Launch above the most: %v", err)
	}
	if _, err := r.Launch(id, ann, 100, 100, 300, now); !errors.Is(err, user.ErrRocketAuto) {
		t.Fatalf("Launch cashing at ×1: %v", err)
	}
	if _, err := r.Launch(id, ann, 100, 100_001, 300, now); !errors.Is(err, user.ErrRocketAuto) {
		t.Fatalf("Launch cashing past the top: %v", err)
	}

	// By hand: it flies until ×3, cashes at what it shows.
	round, err := r.Launch(id, ann, 100, 0, 300, now)
	if err != nil {
		t.Fatal(err)
	}
	if !round.Owned(ann) || round.Owned(uuid.New()) || round.StateAt(now) != user.RocketFlying {
		t.Fatalf("round = %+v", round)
	}

	cashed, err := round.Cash(now.Add(5777 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if cashed.Cashed() != 200 || cashed.Payout() != 200 || cashed.StateAt(now.Add(time.Hour)) != user.RocketCashed {
		t.Fatalf("cashed = %+v", cashed)
	}
	if round.Cashed() != 0 {
		t.Fatal("Cash changed the round")
	}
	if _, err := cashed.Cash(now.Add(6 * time.Second)); !errors.Is(err, user.ErrRocketOver) {
		t.Fatalf("Cash twice: %v", err)
	}

	// Too late: the rocket exploded at ×3, in about 9.2 s.
	if round.StateAt(now.Add(10*time.Second)) != user.RocketCrashed {
		t.Fatal("the rocket flies past its crash")
	}
	late, err := round.Cash(now.Add(10 * time.Second))
	if err != nil || late.Cashed() != 0 || late.Payout() != 0 {
		t.Fatalf("late Cash = %+v, %v", late, err)
	}

	// By itself: cashed at ×2 before ×3, lost at ×4.
	won, err := r.Launch(id, ann, 15, 200, 300, now)
	if err != nil || won.Cashed() != 200 || won.Payout() != 30 || won.StateAt(now) != user.RocketCashed {
		t.Fatalf("auto ×2 = %+v, %v", won, err)
	}
	lost, err := r.Launch(id, ann, 15, 400, 300, now)
	if err != nil || lost.Cashed() != 0 || lost.StateAt(now) != user.RocketCrashed {
		t.Fatalf("auto ×4 = %+v, %v", lost, err)
	}
	if _, err := won.Cash(now); !errors.Is(err, user.ErrRocketOver) {
		t.Fatalf("Cash of auto: %v", err)
	}
}
