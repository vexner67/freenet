package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewPrimeStyle(t *testing.T) {
	t.Parallel()

	s, err := user.NewPrimeStyle("", "", "", "")
	if err != nil || s != user.DefaultPrimeStyle() || !s.IsDefault() {
		t.Fatalf("the default = %+v, %v", s, err)
	}

	s, err = user.NewPrimeStyle("rainbow", "glitter", "stars", "love")
	if err != nil || s.Name != "rainbow" || s.Frame != "glitter" || s.Background != "stars" || s.Mood != "love" || s.IsDefault() {
		t.Fatalf("a dressed page = %+v, %v", s, err)
	}

	for _, bad := range [][4]string{{"pink", "", "", ""}, {"", "wood", "", ""}, {"", "", "space", ""}, {"", "", "", "bored"}, {"Ruby", "", "", ""}} {
		if _, err = user.NewPrimeStyle(bad[0], bad[1], bad[2], bad[3]); !errors.Is(err, user.ErrPrimeStyleInvalid) {
			t.Fatalf("NewPrimeStyle%q: %v", bad, err)
		}
	}
}

func TestUser_PrimeStyle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	dressed, _ := user.NewPrimeStyle("gold", "stars", "hearts", "cool")

	u := newCompleted(t)
	if !u.PrimeStyle().IsDefault() {
		t.Fatalf("a new page is dressed: %+v", u.PrimeStyle())
	}

	// Without Prime only the default style is allowed, and others see none.
	if err := u.CheckPrimeStyle(dressed, now); !errors.Is(err, user.ErrPrimeOnly) {
		t.Fatalf("without prime: %v", err)
	}
	if err := u.CheckPrimeStyle(user.DefaultPrimeStyle(), now); err != nil {
		t.Fatalf("back to the default: %v", err)
	}

	u = u.WithPrime(user.Prime{Until: now.Add(time.Hour)}).WithPrimeStyle(dressed)
	if err := u.CheckPrimeStyle(dressed, now); err != nil {
		t.Fatalf("with prime: %v", err)
	}
	if p := u.PublicProfileFor(u.ID(), user.Ties{}, now); p.PrimeStyle == nil || *p.PrimeStyle != dressed {
		t.Fatalf("the page with prime = %+v", p.PrimeStyle)
	}

	// Prime has ended: the owner keeps the style, others no longer see it.
	later := now.Add(2 * time.Hour)
	if p := u.PublicProfileFor(u.ID(), user.Ties{}, later); p.PrimeStyle != nil || u.PrimeStyle() != dressed {
		t.Fatalf("after prime = %+v, kept %+v", p.PrimeStyle, u.PrimeStyle())
	}
}
