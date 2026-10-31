package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestRandomCardNumber(t *testing.T) {
	t.Parallel()

	seen := map[user.CardNumber]bool{}
	for range 200 {
		n, err := user.RandomCardNumber()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(n.String(), user.CardBIN) {
			t.Fatalf("number %s does not begin with the BIN", n)
		}
		if _, err = user.NewCardNumber(n.String()); err != nil {
			t.Fatalf("NewCardNumber(%s) = %v", n, err)
		}
		if seen[n] {
			t.Fatalf("number %s drawn twice", n)
		}
		seen[n] = true
	}
}

func TestNewCardNumber(t *testing.T) {
	t.Parallel()

	good, err := user.RandomCardNumber()
	if err != nil {
		t.Fatal(err)
	}
	last := good.String()[15]
	wrong := byte('0' + (last-'0'+1)%10)

	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"short", good.String()[:15]},
		{"long", good.String() + "0"},
		{"another BIN", "4" + good.String()[1:]},
		{"a letter", good.String()[:8] + "a" + good.String()[9:]},
		{"wrong check digit", good.String()[:15] + string(wrong)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := user.NewCardNumber(tt.value); !errors.Is(err, user.ErrCardNumber) {
				t.Fatalf("NewCardNumber(%q) = %v", tt.value, err)
			}
		})
	}
}

func TestNewCardCover(t *testing.T) {
	t.Parallel()

	if c, err := user.NewCardCover("gold"); err != nil || c != "gold" {
		t.Fatalf("NewCardCover(gold) = %q, %v", c, err)
	}
	for _, s := range []string{"", "GOLD", "paper"} {
		if _, err := user.NewCardCover(s); !errors.Is(err, user.ErrCardCover) {
			t.Fatalf("NewCardCover(%q) = %v", s, err)
		}
	}
}

func TestNewCard(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	c := user.NewBankCard("1919000000000000", now)
	if c.Cover() != user.DefaultCardCover || !c.IssuedAt().Equal(now) {
		t.Fatalf("NewBankCard = %+v", c)
	}
}
