package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewAlert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want user.Alert
		err  error
	}{
		{name: "words", in: "  Страница под наблюдением  ", want: "Страница под наблюдением"},
		{name: "empty takes it", in: "   ", want: ""},
		{name: "at the limit", in: strings.Repeat("я", 1000), want: user.Alert(strings.Repeat("я", 1000))},
		{name: "too long", in: strings.Repeat("я", 1001), err: user.ErrAlertTooLong},
		{name: "invisible", in: "текст\u200b", err: user.ErrInvisibleText},
		{name: "lines", in: " первый миллионер \r\n\n  олд\n", want: "первый миллионер\nолд"},
		{name: "at the line limit", in: strings.Repeat("я\n", 20), want: user.Alert(strings.TrimSuffix(strings.Repeat("я\n", 20), "\n"))},
		{name: "too many lines", in: strings.Repeat("я\n", 21), err: user.ErrAlertLines},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewAlert(tt.in)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewAlert(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestUser_Alert(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	if u.Alert() != "" || u.PublicProfile().Alert != "" {
		t.Fatal("a new page has a label")
	}

	labeled := u.WithAlert("Тестовая метка")
	if u.Alert() != "" {
		t.Fatal("WithAlert changed the user it was called on")
	}
	if labeled.Alert() != "Тестовая метка" || labeled.PublicProfile().Alert != "Тестовая метка" || labeled.ListedProfile(time.Now()).Alert != "Тестовая метка" {
		t.Fatal("the label is not told")
	}
	if labeled.WithAlert("").PublicProfile().Alert != "" {
		t.Fatal("a taken label is told")
	}
}

func TestUser_AlertHiddenClosedAndDeleted(t *testing.T) {
	t.Parallel()

	hidden := user.Restore(user.Stored{
		ID:      uuid.New(),
		Privacy: user.Privacy{PageRead: user.AudienceUsers, InfoRead: user.AudienceOnlyMe},
		Alert:   "Метка",
	})
	if p := hidden.PublicProfileFor(uuid.Nil, user.Ties{}, time.Now()); !p.PageHidden || p.Alert != "Метка" {
		t.Fatalf("a hidden page: %+v", p)
	}
	if p := hidden.PublicProfile(); !p.InfoHidden || p.Alert != "Метка" {
		t.Fatalf("a page with hidden information: %+v", p)
	}

	deleted := user.Restore(user.Stored{
		ID:           uuid.New(),
		Deactivation: user.Deactivation{At: time.Now(), PurgeAt: time.Now().Add(time.Hour)},
		Alert:        "Метка",
	})
	if deleted.PublicProfile().Alert != "" || deleted.PublicProfileFor(uuid.Nil, user.Ties{}, time.Now()).Alert != "" {
		t.Fatal("a deleted page tells the label")
	}
}
