package user

import (
	"errors"
	"slices"
)

var (
	ErrInvalidAvatarStyle = errors.New("invalid avatar style")
	ErrInvalidMainPage    = errors.New("invalid main page")
	ErrInvalidMenuItem    = errors.New("invalid menu item")
)

// AvatarStyle is how the owner sees everyone's photos ("Отображение
// аватаров" of openvk).
type AvatarStyle string

const (
	// AvatarsSquare is as they are stored, the default.
	AvatarsSquare AvatarStyle = "square"
	AvatarsRound  AvatarStyle = "round"
)

func NewAvatarStyle(s string) (AvatarStyle, error) {
	switch AvatarStyle(s) {
	case AvatarsSquare, AvatarsRound:
		return AvatarStyle(s), nil
	default:
		return "", ErrInvalidAvatarStyle
	}
}

func (a AvatarStyle) String() string {
	return string(a)
}

// MainPage is where the site opens for its owner ("Главная страница" of
// openvk): their page, the default, or the news.
type MainPage string

const (
	MainPagePage MainPage = "page"
	MainPageFeed MainPage = "feed"
)

func NewMainPage(s string) (MainPage, error) {
	switch MainPage(s) {
	case MainPagePage, MainPageFeed:
		return MainPage(s), nil
	default:
		return "", ErrInvalidMainPage
	}
}

func (m MainPage) String() string {
	return string(m)
}

// MenuItem is a link of the left menu its owner may take away ("Левое
// меню" of openvk). The page itself and the settings always stay.
type MenuItem string

const (
	MenuPhotos        MenuItem = "photos"
	MenuMessages      MenuItem = "messages"
	MenuGroups        MenuItem = "groups"
	MenuEvents        MenuItem = "events"
	MenuFeed          MenuItem = "feed"
	MenuNotifications MenuItem = "notifications"
)

// menuItems are all of them, in the order of the menu.
var menuItems = []MenuItem{MenuPhotos, MenuMessages, MenuGroups, MenuEvents, MenuFeed, MenuNotifications}

func (m MenuItem) String() string {
	return string(m)
}

// Interface is how the site looks to its owner ("Интерфейс" of openvk's
// settings): nobody else sees or is touched by it.
type Interface struct {
	AvatarStyle AvatarStyle
	MainPage    MainPage
	// HiddenMenu are the links taken out of the left menu, each once, in
	// the order of the menu.
	HiddenMenu []MenuItem
}

// DefaultInterface is the site as a new user sees it.
func DefaultInterface() Interface {
	return Interface{AvatarStyle: AvatarsSquare, MainPage: MainPagePage}
}

// NewInterface reads the settings form: every value is one of those the
// site knows, and a link hidden twice is hidden once.
func NewInterface(avatarStyle, mainPage string, hiddenMenu []string) (Interface, error) {
	style, err := NewAvatarStyle(avatarStyle)
	if err != nil {
		return Interface{}, err
	}

	main, err := NewMainPage(mainPage)
	if err != nil {
		return Interface{}, err
	}

	for _, item := range hiddenMenu {
		if !slices.Contains(menuItems, MenuItem(item)) {
			return Interface{}, ErrInvalidMenuItem
		}
	}

	var hidden []MenuItem
	for _, item := range menuItems {
		if slices.Contains(hiddenMenu, item.String()) {
			hidden = append(hidden, item)
		}
	}

	return Interface{AvatarStyle: style, MainPage: main, HiddenMenu: hidden}, nil
}

// Interface is how the site looks to the user.
func (u *User) Interface() Interface {
	return u.ui
}

// UpdateInterface replaces the settings of "Интерфейс".
func (u *User) UpdateInterface(ui Interface) {
	u.ui = ui
}
