package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewBackdropSide(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"left", "right", "cover"} {
		if got, err := user.NewBackdropSide(s); err != nil || got.String() != s {
			t.Errorf("%s: %v, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "top", "LEFT"} {
		if _, err := user.NewBackdropSide(s); err != user.ErrInvalidBackdropSide {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestUser_Backdrop(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	_ = u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann"})
	left, right, other := uuid.New(), uuid.New(), uuid.New()

	if prev := u.SetBackdrop(user.BackdropLeft, left); prev != uuid.Nil {
		t.Fatalf("first left replaced %v", prev)
	}
	u.SetBackdrop(user.BackdropRight, right)
	if prev := u.SetBackdrop(user.BackdropLeft, other); prev != left {
		t.Fatalf("replaced %v, want %v", prev, left)
	}

	want := user.Backdrop{Left: other, Right: right}
	if u.Backdrop() != want || u.PublicProfile().Backdrop != want {
		t.Fatalf("backdrop %+v, public %+v", u.Backdrop(), u.PublicProfile().Backdrop)
	}

	// A deleted page shows nothing but the names and the photo.
	_ = u.Deactivate("", time.Now(), time.Hour)
	if got := u.PublicProfile().Backdrop; got != (user.Backdrop{}) {
		t.Fatalf("deleted page: %+v", got)
	}
}

func TestUser_BackdropCover(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	photo := uuid.New()
	u := newCompleted(t)

	// Without Prime a photo behind the page is refused, taking it away
	// and the sides are not.
	if err := u.CheckBackdrop(user.BackdropCover, photo, now); !errors.Is(err, user.ErrPrimeOnly) {
		t.Fatalf("without prime: %v", err)
	}
	for _, ok := range []struct {
		side  user.BackdropSide
		photo uuid.UUID
	}{{user.BackdropCover, uuid.Nil}, {user.BackdropLeft, photo}} {
		if err := u.CheckBackdrop(ok.side, ok.photo, now); err != nil {
			t.Fatalf("%s %v: %v", ok.side, ok.photo, err)
		}
	}

	u = u.WithPrime(user.Prime{Until: now.Add(time.Hour)})
	if err := u.CheckBackdrop(user.BackdropCover, photo, now); err != nil {
		t.Fatalf("with prime: %v", err)
	}
	u.SetBackdrop(user.BackdropCover, photo)
	if p := u.PublicProfileFor(uuid.Nil, user.Ties{}, now); p.Backdrop.Cover != photo {
		t.Fatalf("the page with prime = %+v", p.Backdrop)
	}

	// Prime has ended: the owner keeps the photo, others no longer see it.
	if p := u.PublicProfileFor(uuid.Nil, user.Ties{}, now.Add(2*time.Hour)); p.Backdrop.Cover != uuid.Nil || u.Backdrop().Cover != photo {
		t.Fatalf("after prime = %+v, kept %+v", p.Backdrop, u.Backdrop())
	}
}
