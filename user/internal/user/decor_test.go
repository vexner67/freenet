package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewDecor(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"auto", "on", "off"} {
		if d, err := user.NewDecor(s); err != nil || d.String() != s {
			t.Fatalf("%q: %q, %v", s, d, err)
		}
	}
	for _, s := range []string{"", "On", "snow"} {
		if _, err := user.NewDecor(s); !errors.Is(err, user.ErrDecorInvalid) {
			t.Fatalf("%q: %v", s, err)
		}
	}
}

func TestDecor_FestiveAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		decor user.Decor
		at    time.Time
		want  bool
	}{
		{name: "autumn", decor: user.DecorAuto, at: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)},
		{name: "the eve of the season", decor: user.DecorAuto, at: time.Date(2026, 12, 14, 20, 59, 0, 0, time.UTC)},
		{name: "the season comes in Moscow", decor: user.DecorAuto, at: time.Date(2026, 12, 14, 21, 0, 0, 0, time.UTC), want: true},
		{name: "the New Year", decor: user.DecorAuto, at: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), want: true},
		{name: "the old New Year", decor: user.DecorAuto, at: time.Date(2027, 1, 14, 20, 0, 0, 0, time.UTC), want: true},
		{name: "the season is over", decor: user.DecorAuto, at: time.Date(2027, 1, 14, 21, 0, 0, 0, time.UTC)},
		{name: "on in autumn", decor: user.DecorOn, at: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), want: true},
		{name: "off on the New Year", decor: user.DecorOff, at: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.decor.FestiveAt(tt.at); got != tt.want {
				t.Fatalf("FestiveAt = %v; want %v", got, tt.want)
			}
		})
	}
}
