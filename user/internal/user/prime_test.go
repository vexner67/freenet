package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPrimePeriod(t *testing.T) {
	t.Parallel()

	for s, days := range map[string]int{"month": 30, "quarter": 90, "year": 365} {
		p, err := user.NewPrimePeriod(s)
		if err != nil || p.Days() != days || p.String() != s {
			t.Fatalf("NewPrimePeriod(%q) = %v (%d days), %v", s, p, p.Days(), err)
		}
	}

	if _, err := user.NewPrimePeriod("week"); !errors.Is(err, user.ErrPrimePeriodInvalid) {
		t.Fatalf("a week: %v", err)
	}
}

func TestPrimePrices(t *testing.T) {
	t.Parallel()

	p, err := user.NewPrimePrices(1, 2, user.MaxPrimePrice)
	if err != nil || p.Of(user.PrimeMonth) != 1 || p.Of(user.PrimeQuarter) != 2 || p.Of(user.PrimeYear) != user.MaxPrimePrice {
		t.Fatalf("NewPrimePrices = %+v, %v", p, err)
	}

	for _, bad := range [][3]int64{{0, 1, 1}, {1, -1, 1}, {1, 1, user.MaxPrimePrice + 1}} {
		if _, err = user.NewPrimePrices(bad[0], bad[1], bad[2]); !errors.Is(err, user.ErrPrimePriceInvalid) {
			t.Fatalf("NewPrimePrices%v: %v", bad, err)
		}
	}
}

func TestPrime_Extended(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

	// Never bought or ended: from now.
	if got := (user.Prime{}).Extended(user.PrimeMonth, now); !got.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("from nothing = %v", got)
	}
	ended := user.Prime{Until: now.Add(-time.Hour)}
	if got := ended.Extended(user.PrimeMonth, now); !got.Equal(now.AddDate(0, 0, 30)) || ended.ActiveAt(now) {
		t.Fatalf("after the end = %v", got)
	}

	// Lasting: added to its end.
	lasting := user.Prime{Until: now.AddDate(0, 0, 10)}
	if got := lasting.Extended(user.PrimeYear, now); !got.Equal(now.AddDate(0, 0, 375)) || !lasting.ActiveAt(now) {
		t.Fatalf("on top = %v", got)
	}
}

func TestUser_Prime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	plain := user.Restore(user.Stored{ID: uuid.New()})
	prime := plain.WithPrime(user.Prime{Until: now.Add(time.Hour)})

	if plain.IsPrimeAt(now) || !prime.IsPrimeAt(now) || prime.IsPrimeAt(now.Add(time.Hour)) {
		t.Fatal("IsPrimeAt")
	}

	// A deleted page has no Prime.
	deleted := user.Restore(user.Stored{ID: uuid.New(), Prime: prime.Prime(), Deactivation: user.Deactivation{At: now, PurgeAt: now.Add(time.Hour)}})
	if deleted.IsPrimeAt(now) {
		t.Fatal("a deleted page has Prime")
	}

	if err := plain.CheckInvisible(true, now); !errors.Is(err, user.ErrPrimeOnly) {
		t.Fatalf("invisible without Prime: %v", err)
	}
	if plain.CheckInvisible(false, now) != nil || prime.CheckInvisible(true, now) != nil {
		t.Fatal("CheckInvisible refused what is allowed")
	}

	if plain.TransferFeeAt(100, now) != user.TransferFee(100) || prime.TransferFeeAt(100, now) != 0 {
		t.Fatal("TransferFeeAt")
	}
	if plain.StickerTaxAt(10, now) != user.StickerTax(10) || prime.StickerTaxAt(100, now) != 5 || prime.StickerTaxAt(10, now) != 1 {
		t.Fatalf("StickerTaxAt = %d, %d", prime.StickerTaxAt(100, now), prime.StickerTaxAt(10, now))
	}
}

func TestUser_PrimeInvisible(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	ann, _ := user.New("ann@example.com")
	_ = ann.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann"})
	ann.MarkSeen(now.Add(-time.Minute))

	shown := ann.WithPrime(user.Prime{Until: now.Add(time.Hour)})
	if p := shown.PublicProfileFor(uuid.Nil, user.Ties{}, now); !p.Online || !p.Prime {
		t.Fatalf("Prime, visible: %+v", p)
	}

	hidden := ann.WithPrime(user.Prime{Until: now.Add(time.Hour), Invisible: true})
	if p := hidden.PublicProfileFor(uuid.New(), user.Ties{}, now); p.Online || !p.LastSeen.IsZero() {
		t.Fatalf("invisible to others: %+v", p)
	}
	if p := hidden.ListedProfile(now); p.Online || !p.Prime {
		t.Fatalf("invisible in lists: %+v", p)
	}
	if p := hidden.PublicProfileFor(hidden.ID(), user.Ties{}, now); !p.Online {
		t.Fatalf("invisible to oneself: %+v", p)
	}

	// Prime over, the page shows again.
	if p := hidden.PublicProfileFor(uuid.New(), user.Ties{}, now.Add(2*time.Hour)); p.LastSeen.IsZero() || p.Prime {
		t.Fatalf("after Prime: %+v", p)
	}
}
