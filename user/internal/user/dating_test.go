package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewDatingCard(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		about string
		want  string
		err   error
	}{
		{"empty", "", "", nil},
		{"trimmed", "  люблю котов  ", "люблю котов", nil},
		{"longest", strings.Repeat("я", user.MaxDatingAbout), strings.Repeat("я", user.MaxDatingAbout), nil},
		{"too long", strings.Repeat("я", user.MaxDatingAbout+1), "", user.ErrDatingAbout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := user.NewDatingCard(uuid.New(), user.DatingChat, user.GenderNone, tt.about, true, now)
			if !errors.Is(err, tt.err) || c.About != tt.want {
				t.Fatalf("NewDatingCard = %q, %v", c.About, err)
			}
		})
	}
}

func TestDatingValues(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"friendship", "chat", "love"} {
		if g, err := user.NewDatingGoal(s); err != nil || g.String() != s {
			t.Fatalf("NewDatingGoal(%q) = %q, %v", s, g, err)
		}
	}
	if _, err := user.NewDatingGoal("money"); !errors.Is(err, user.ErrDatingGoal) {
		t.Fatalf("NewDatingGoal(money): %v", err)
	}
	for _, s := range []string{"", "male", "female"} {
		if _, err := user.NewDatingSeek(s); err != nil {
			t.Fatalf("NewDatingSeek(%q): %v", s, err)
		}
	}
	if _, err := user.NewDatingSeek("neutral"); !errors.Is(err, user.ErrDatingSeek) {
		t.Fatalf("NewDatingSeek(neutral): %v", err)
	}
}

func TestDatingCardSuits(t *testing.T) {
	t.Parallel()

	card := func(seek user.Gender, on bool) user.DatingCard {
		return user.DatingCard{UserID: uuid.New(), Seek: seek, On: on}
	}
	tests := []struct {
		name         string
		mine, theirs user.DatingCard
		me, them     user.Gender
		want         bool
	}{
		{"anyone and anyone", card(user.GenderNone, true), card(user.GenderNone, true), user.GenderMale, user.GenderNeutral, true},
		{"each seeks the other", card(user.GenderFemale, true), card(user.GenderMale, true), user.GenderMale, user.GenderFemale, true},
		{"I seek another", card(user.GenderMale, true), card(user.GenderNone, true), user.GenderMale, user.GenderFemale, false},
		{"they seek another", card(user.GenderFemale, true), card(user.GenderFemale, true), user.GenderMale, user.GenderFemale, false},
		{"theirs is off", card(user.GenderNone, true), card(user.GenderNone, false), user.GenderMale, user.GenderFemale, false},
		{"mine is off", card(user.GenderNone, false), card(user.GenderNone, true), user.GenderMale, user.GenderFemale, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.mine.Suits(tt.me, tt.theirs, tt.them); got != tt.want {
				t.Fatalf("Suits = %v, want %v", got, tt.want)
			}
		})
	}
	if c := card(user.GenderNone, true); c.Suits(user.GenderMale, c, user.GenderMale) {
		t.Fatal("a card suits itself")
	}
}
