package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewVoucher_UnlimitedVotes(t *testing.T) {
	t.Parallel()

	token, err := user.GenerateVoucherToken()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := user.NewVoucher(token, 0, 10, user.UnlimitedUsages); err != nil {
		t.Fatalf("unlimited rating: %v", err)
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
