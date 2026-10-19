package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewBanReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want error
	}{
		{"plain", "  спам  ", nil},
		{"empty", "   ", user.ErrBanReasonEmpty},
		{"longest", strings.Repeat("я", 255), nil},
		{"too long", strings.Repeat("я", 256), user.ErrBanReasonTooLong},
		{"invisible", "ㅤ", user.ErrInvisibleText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := user.NewBanReason(tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if err == nil && r.String() != strings.TrimSpace(tt.in) {
				t.Fatalf("reason = %q", r)
			}
		})
	}
}

func TestNewBan(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	if _, err := user.NewBan("", now, time.Time{}); !errors.Is(err, user.ErrBanReasonEmpty) {
		t.Fatalf("no reason: %v", err)
	}
	if _, err := user.NewBan("spam", now, now); !errors.Is(err, user.ErrBanUntilPast) {
		t.Fatalf("ends at once: %v", err)
	}

	forever, err := user.NewBan("spam", now, time.Time{})
	if err != nil || !forever.Forever() {
		t.Fatalf("for good: %+v, %v", forever, err)
	}
	if !forever.ActiveAt(now.AddDate(100, 0, 0)) {
		t.Fatal("a ban for good ends")
	}

	day, err := user.NewBan("spam", now, now.Add(24*time.Hour))
	if err != nil || day.Forever() {
		t.Fatalf("for a day: %+v, %v", day, err)
	}
	if !day.ActiveAt(now.Add(time.Hour)) || day.ActiveAt(now.Add(24*time.Hour)) {
		t.Fatal("a ban for a day is active at the wrong times")
	}
	if (user.Ban{}).ActiveAt(now) {
		t.Fatal("the zero ban blocks")
	}
}

func TestUser_Ban(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	ann, _ := user.New("ann@example.com")
	_ = ann.UpdateProfile(user.Profile{Name: "Анна", Surname: "Иванова", Nickname: "ann", Hometown: "Тверь"})

	ban, _ := user.NewBan("spam", now, now.Add(time.Hour))
	banned := ann.WithBan(ban)

	if ann.IsBannedAt(now) || !banned.IsBannedAt(now) {
		t.Fatal("WithBan changed the user it was called on, or did not block")
	}
	if banned.BanAt(now.Add(2*time.Hour)) != (user.Ban{}) {
		t.Fatal("an ended ban is told")
	}

	// Others see the name, the photo and the ban; the owner sees the page.
	p := banned.PublicProfileFor(uuid.New(), user.Ties{}, now)
	if p.Name != "Анна" || p.Hometown != "" || !p.InfoHidden || p.Ban != ban {
		t.Fatalf("others see %+v", p)
	}
	if p := banned.PublicProfileFor(ann.ID(), user.Ties{}, now); p.Hometown != "Тверь" {
		t.Fatalf("the owner sees %+v", p)
	}
	if a := banned.AccessFor(uuid.New(), user.Ties{Followed: true}, now); a.Info || a.Photos || !a.Page {
		t.Fatalf("access of a blocked page: %+v", a)
	}
	if p := banned.ListedProfile(now); p.Hometown != "" || p.Ban != ban {
		t.Fatalf("listed: %+v", p)
	}
	if p := banned.ListedProfile(now.Add(2 * time.Hour)); p.Ban != (user.Ban{}) {
		t.Fatalf("listed after the ban: %+v", p)
	}

	if err := ann.CheckBanBy(ann.ID(), false); !errors.Is(err, user.ErrBanSelf) {
		t.Fatalf("self: %v", err)
	}
	if err := ann.CheckBanBy(uuid.New(), true); !errors.Is(err, user.ErrBanAdmin) {
		t.Fatalf("an administrator: %v", err)
	}
	if err := ann.CheckBanBy(uuid.New(), false); err != nil {
		t.Fatalf("anyone else: %v", err)
	}
}
