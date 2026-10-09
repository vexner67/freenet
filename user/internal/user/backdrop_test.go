package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewBackdropSide(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"left", "right"} {
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
