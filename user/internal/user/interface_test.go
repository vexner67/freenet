package user_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

// defaultForm is the settings form of a new user.
func defaultForm() user.InterfaceForm {
	return user.InterfaceForm{AvatarStyle: "square", Theme: "default", NSFW: "blur", PostView: "old", MainPage: "page"}
}

func TestNewInterface(t *testing.T) {
	t.Parallel()

	moscow, err := user.NewTimezone(180)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		change func(*user.InterfaceForm)
		want   user.Interface
		err    error
	}{
		{"as a new user has it", func(*user.InterfaceForm) {}, user.DefaultInterface(), nil},
		{
			"everything changed; the menu in its order and once each",
			func(f *user.InterfaceForm) {
				offset := 180
				*f = user.InterfaceForm{
					AvatarStyle: "round", Theme: "dark", HideRating: true, NSFW: "show", PostView: "microblog",
					MainPage: "feed", Timezone: &offset, HiddenMenu: []string{"notifications", "photos", "photos"},
				}
			},
			user.Interface{
				AvatarStyle: user.AvatarsRound, Theme: user.ThemeDark, HideRating: true, NSFW: user.NSFWShow,
				PostView: user.PostViewMicroblog, MainPage: user.MainPageFeed, Timezone: moscow,
				HiddenMenu: []user.MenuItem{user.MenuPhotos, user.MenuNotifications},
			},
			nil,
		},
		{"an unknown style", func(f *user.InterfaceForm) { f.AvatarStyle = "oval" }, user.Interface{}, user.ErrInvalidAvatarStyle},
		{"no style", func(f *user.InterfaceForm) { f.AvatarStyle = "" }, user.Interface{}, user.ErrInvalidAvatarStyle},
		{"an unknown theme", func(f *user.InterfaceForm) { f.Theme = "pink" }, user.Interface{}, user.ErrInvalidTheme},
		{"an unknown nsfw setting", func(f *user.InterfaceForm) { f.NSFW = "maybe" }, user.Interface{}, user.ErrInvalidNSFW},
		{"an unknown view of posts", func(f *user.InterfaceForm) { f.PostView = "blog" }, user.Interface{}, user.ErrInvalidPostView},
		{"an unknown main page", func(f *user.InterfaceForm) { f.MainPage = "search" }, user.Interface{}, user.ErrInvalidMainPage},
		{"a clock off the map", func(f *user.InterfaceForm) { f.Timezone = ptr(900) }, user.Interface{}, user.ErrInvalidTimezone},
		{"a clock between the quarters", func(f *user.InterfaceForm) { f.Timezone = ptr(7) }, user.Interface{}, user.ErrInvalidTimezone},
		{"the settings cannot be hidden", func(f *user.InterfaceForm) { f.HiddenMenu = []string{"settings"} }, user.Interface{}, user.ErrInvalidMenuItem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			form := defaultForm()
			tt.change(&form)

			got, err := user.NewInterface(form)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewInterface = %v, want %v", err, tt.err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NewInterface = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewTimezone(t *testing.T) {
	t.Parallel()

	if _, fixed := (user.Timezone{}).Offset(); fixed {
		t.Fatal("the browser's clock is a fixed one")
	}

	for _, minutes := range []int{-720, -210, 0, 345, 840} {
		zone, err := user.NewTimezone(minutes)
		if got, fixed := zone.Offset(); err != nil || !fixed || got != minutes {
			t.Errorf("NewTimezone(%d) = %d, %v, %v", minutes, got, fixed, err)
		}
	}
	for _, minutes := range []int{-735, 855, 1, -44} {
		if _, err := user.NewTimezone(minutes); !errors.Is(err, user.ErrInvalidTimezone) {
			t.Errorf("NewTimezone(%d) = %v", minutes, err)
		}
	}
}

func TestUser_Interface(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Interface(); !reflect.DeepEqual(got, user.DefaultInterface()) {
		t.Fatalf("a new user = %+v", got)
	}

	form := defaultForm()
	form.Theme, form.HiddenMenu = "blue", []string{"events"}

	ui, err := user.NewInterface(form)
	if err != nil {
		t.Fatal(err)
	}
	u.UpdateInterface(ui)
	if got := u.Interface(); !reflect.DeepEqual(got, ui) || got.Theme != user.ThemeBlue {
		t.Fatalf("after the change = %+v", got)
	}
}

func ptr(n int) *int {
	return &n
}
