package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestTreasury(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		treasury user.Treasury
		free     int64
		bonus    int64
		maxWin   int64
	}{
		{name: "the start", treasury: user.Treasury{Votes: user.TreasuryStart}, free: 5000, bonus: 10, maxWin: 1000},
		{name: "promised", treasury: user.Treasury{Votes: user.TreasuryStart, Outstanding: 4000}, free: 1000, bonus: 10, maxWin: 1000},
		{name: "at the reserve", treasury: user.Treasury{Votes: user.TreasuryReserve}, free: 0, bonus: 10, maxWin: 1000},
		{name: "half the reserve", treasury: user.Treasury{Votes: 10_000}, free: 0, bonus: 5, maxWin: 500},
		{name: "empty", treasury: user.Treasury{}, free: 0, bonus: 1, maxWin: 0},
	}

	for _, tt := range tests {
		if got := tt.treasury.Free(); got != tt.free {
			t.Errorf("%s: Free = %d, want %d", tt.name, got, tt.free)
		}
		if got := tt.treasury.Bonus(); got != tt.bonus {
			t.Errorf("%s: Bonus = %d, want %d", tt.name, got, tt.bonus)
		}
		if got := tt.treasury.MaxWin(); got != tt.maxWin {
			t.Errorf("%s: MaxWin = %d, want %d", tt.name, got, tt.maxWin)
		}
	}

	half := user.Treasury{Votes: 10_000}
	if half.CheckWin(500) != nil || !errors.Is(half.CheckWin(501), user.ErrTreasuryLow) {
		t.Fatal("CheckWin does not stop at 1/20 of the treasury")
	}
}

func TestTreasury_CheckVoucher(t *testing.T) {
	t.Parallel()

	token, err := user.GenerateVoucherToken()
	if err != nil {
		t.Fatal(err)
	}

	treasury := user.Treasury{Votes: user.TreasuryStart, Outstanding: 1000}

	fits, err := user.NewVoucher(token, 100, 0, 40)
	if err != nil {
		t.Fatal(err)
	}
	if err := treasury.CheckVoucher(fits); err != nil {
		t.Fatalf("CheckVoucher of 4000: %v", err)
	}

	big, err := user.NewVoucher(token, 100, 0, 41)
	if err != nil {
		t.Fatal(err)
	}
	if err := treasury.CheckVoucher(big); !errors.Is(err, user.ErrVoucherCover) {
		t.Fatalf("CheckVoucher of 4100: %v", err)
	}

	// Rating costs the treasury nothing.
	rating, err := user.NewVoucher(token, 0, 10, user.UnlimitedUsages)
	if err != nil {
		t.Fatal(err)
	}
	if err := (user.Treasury{}).CheckVoucher(rating); err != nil {
		t.Fatalf("CheckVoucher of rating: %v", err)
	}

	if _, err := user.NewVoucher(token, 1, 0, user.UnlimitedUsages); !errors.Is(err, user.ErrVoucherUnlimitedVotes) {
		t.Fatalf("unlimited votes: %v", err)
	}
}

func TestBonusDay(t *testing.T) {
	t.Parallel()

	// 21:00 UTC is midnight in Moscow: the next day begins.
	if got := user.BonusDay(time.Date(2026, 10, 21, 20, 59, 0, 0, time.UTC)); !got.Equal(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("BonusDay before midnight = %v", got)
	}
	if got := user.BonusDay(time.Date(2026, 10, 21, 21, 0, 0, 0, time.UTC)); !got.Equal(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("BonusDay at midnight = %v", got)
	}
}

func TestUser_CheckBonusAt(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	week := user.RegisteredAt(u.ID()).Add(user.BonusAge)
	if err := u.CheckBonusAt(week); !errors.Is(err, user.ErrBonusNotYet) {
		t.Fatalf("CheckBonusAt without a profile: %v", err)
	}

	if err = u.UpdateProfile(user.Profile{Name: "Анна", Nickname: "ann"}); err != nil {
		t.Fatal(err)
	}
	if err := u.CheckBonusAt(week.Add(-time.Second)); !errors.Is(err, user.ErrBonusNotYet) {
		t.Fatalf("CheckBonusAt before a week: %v", err)
	}
	if err := u.CheckBonusAt(week); err != nil {
		t.Fatalf("CheckBonusAt after a week: %v", err)
	}
}
