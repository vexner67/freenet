package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewDeactivationReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want user.DeactivationReason
		err  error
	}{
		{"empty", "", "", nil},
		{"trimmed, line breaks normalised", "  Сайт отнимает время\r\nпока 👋 ", "Сайт отнимает время\nпока 👋", nil},
		{"longest", strings.Repeat("я", 1000), user.DeactivationReason(strings.Repeat("я", 1000)), nil},
		{"too long", strings.Repeat("я", 1001), "", user.ErrDeactivationReasonTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewDeactivationReason(tt.in)
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Fatalf("got %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func newCompleted(t *testing.T) *user.User {
	t.Helper()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	profile := user.Profile{Name: "Анна", Surname: "Петрова", Nickname: "ann", Status: "привет", Gender: user.GenderFemale}
	if err = u.UpdateProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err = u.UpdatePrivacy(user.Privacy{WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone}); err != nil {
		t.Fatal(err)
	}

	return u
}

func TestUser_DeactivateAndReactivate(t *testing.T) {
	t.Parallel()

	u := newCompleted(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	keep := 7 * 30 * 24 * time.Hour

	if err := u.Reactivate(now); !errors.Is(err, user.ErrNotDeactivated) {
		t.Fatalf("Reactivate an active page: %v", err)
	}

	if err := u.Deactivate("надоело", now, keep); err != nil {
		t.Fatal(err)
	}
	if d := u.Deactivation(); !u.IsDeactivated() || !d.At.Equal(now) || !d.PurgeAt.Equal(now.Add(keep)) || d.Reason != "надоело" {
		t.Fatalf("deactivation = %+v", d)
	}
	if err := u.Deactivate("", now, keep); !errors.Is(err, user.ErrDeactivated) {
		t.Fatalf("Deactivate twice: %v", err)
	}

	// Others see the name and the photo only.
	p := u.PublicProfile()
	if !p.Deactivated || p.Name != "Анна" || p.Surname != "Петрова" || p.Nickname != "ann" || p.Status != "" || p.Gender != "" {
		t.Fatalf("public profile of a deleted page = %+v", p)
	}

	// Nobody writes on its wall, its owner included, nor to it.
	other := uuid.New()
	if u.CanPostOnWall(u.ID(), user.Ties{}) || u.CanPostOnWall(other, user.Ties{}) {
		t.Fatal("a deleted page has a wall")
	}
	if u.CanBeMessagedBy(other, user.Ties{WroteFirst: true}) {
		t.Fatal("a deleted page can be written to")
	}

	if err := u.Reactivate(now.Add(keep)); !errors.Is(err, user.ErrRestoreExpired) {
		t.Fatalf("Reactivate when due to be erased: %v", err)
	}

	if err := u.Reactivate(now.Add(keep - time.Second)); err != nil {
		t.Fatalf("Reactivate in time: %v", err)
	}
	if u.IsDeactivated() || u.PublicProfile().Status != "привет" || !u.CanPostOnWall(other, user.Ties{}) {
		t.Fatal("the restored page is not as before")
	}
}
