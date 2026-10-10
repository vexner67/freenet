package user_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewInterface(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		avatars, main string
		hidden        []string
		want          user.Interface
		err           error
	}{
		{"as a new user has it", "square", "page", nil, user.DefaultInterface(), nil},
		{
			"round photos, the news first and a short menu, in its order and once each",
			"round", "feed", []string{"notifications", "photos", "photos"},
			user.Interface{AvatarStyle: user.AvatarsRound, MainPage: user.MainPageFeed, HiddenMenu: []user.MenuItem{user.MenuPhotos, user.MenuNotifications}},
			nil,
		},
		{"an unknown style", "oval", "page", nil, user.Interface{}, user.ErrInvalidAvatarStyle},
		{"no style", "", "page", nil, user.Interface{}, user.ErrInvalidAvatarStyle},
		{"an unknown main page", "square", "search", nil, user.Interface{}, user.ErrInvalidMainPage},
		{"the settings cannot be hidden", "square", "page", []string{"settings"}, user.Interface{}, user.ErrInvalidMenuItem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewInterface(tt.avatars, tt.main, tt.hidden)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewInterface = %v, want %v", err, tt.err)
			}
			if got.AvatarStyle != tt.want.AvatarStyle || got.MainPage != tt.want.MainPage || !slices.Equal(got.HiddenMenu, tt.want.HiddenMenu) {
				t.Fatalf("NewInterface = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUser_Interface(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Interface(); got.AvatarStyle != user.AvatarsSquare || got.MainPage != user.MainPagePage || len(got.HiddenMenu) != 0 {
		t.Fatalf("a new user = %+v", got)
	}

	ui, _ := user.NewInterface("round", "feed", []string{"events"})
	u.UpdateInterface(ui)
	if got := u.Interface(); got.AvatarStyle != user.AvatarsRound || got.MainPage != user.MainPageFeed || !slices.Equal(got.HiddenMenu, []user.MenuItem{user.MenuEvents}) {
		t.Fatalf("after the change = %+v", got)
	}
}
