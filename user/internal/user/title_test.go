package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewTitleName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{name: "plain", in: " Лучший игрок недели ", want: "Лучший игрок недели"},
		{name: "emoji", in: "Король шахты ⛏", want: "Король шахты ⛏"},
		{name: "longest", in: strings.Repeat("я", 40), want: strings.Repeat("я", 40)},
		{name: "empty", in: "  ", err: user.ErrTitleNameEmpty},
		{name: "lines", in: "Король\nшахты", err: user.ErrTitleNameLines},
		{name: "long", in: strings.Repeat("я", 41), err: user.ErrTitleNameTooLong},
		{name: "invisible", in: "Корольㅤшахты", err: user.ErrInvisibleText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewTitleName(tt.in)
			if !errors.Is(err, tt.err) || got.String() != tt.want {
				t.Fatalf("NewTitleName(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestNewTitleIcon(t *testing.T) {
	t.Parallel()

	for _, icon := range user.TitleIcons {
		if got, err := user.NewTitleIcon(string(icon)); err != nil || got != icon {
			t.Fatalf("NewTitleIcon(%q) = %q, %v", icon, got, err)
		}
	}

	for _, in := range []string{"", "Crown", "skull"} {
		if _, err := user.NewTitleIcon(in); !errors.Is(err, user.ErrUnknownTitleIcon) {
			t.Fatalf("NewTitleIcon(%q): %v", in, err)
		}
	}
}

func TestNewTitle(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))

	title, err := user.NewTitle(owner, "Король шахты", user.TitleCrown, now)
	if err != nil || title.ID() == uuid.Nil || title.ID().Version() != 7 || title.UserID() != owner ||
		title.Name() != "Король шахты" || title.Icon() != user.TitleCrown || !title.GrantedAt().Equal(now) || title.GrantedAt().Location() != time.UTC {
		t.Fatalf("NewTitle = %+v, %v", title, err)
	}
}
