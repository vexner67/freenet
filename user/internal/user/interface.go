package user

import (
	"errors"
	"slices"
)

var (
	ErrInvalidAvatarStyle = errors.New("invalid avatar style")
	ErrInvalidMainPage    = errors.New("invalid main page")
	ErrInvalidMenuItem    = errors.New("invalid menu item")
	ErrInvalidTheme       = errors.New("invalid theme")
	ErrInvalidNSFW        = errors.New("invalid nsfw setting")
	ErrInvalidPostView    = errors.New("invalid view of posts")
	ErrInvalidTimezone    = errors.New("invalid timezone")
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
	MenuNotes         MenuItem = "notes"
	MenuGroups        MenuItem = "groups"
	MenuEvents        MenuItem = "events"
	MenuFeed          MenuItem = "feed"
	MenuNotifications MenuItem = "notifications"
)

// menuItems are all of them, in the order of the menu.
var menuItems = []MenuItem{MenuPhotos, MenuMessages, MenuNotes, MenuGroups, MenuEvents, MenuFeed, MenuNotifications}

func (m MenuItem) String() string {
	return string(m)
}

// Theme is the look of the whole site ("Стиль" of openvk).
type Theme string

const (
	ThemeDefault Theme = "default"
	ThemeBlue    Theme = "blue"
	ThemeDark    Theme = "dark"
)

func NewTheme(s string) (Theme, error) {
	switch Theme(s) {
	case ThemeDefault, ThemeBlue, ThemeDark:
		return Theme(s), nil
	default:
		return "", ErrInvalidTheme
	}
}

func (t Theme) String() string {
	return string(t)
}

// NSFW is what the owner sees of posts marked as adult ("NSFW-контент"
// of openvk): blurred until clicked, the default; blurred and left out
// of the news; or as any other post.
type NSFW string

const (
	NSFWBlur NSFW = "blur"
	NSFWHide NSFW = "hide"
	NSFWShow NSFW = "show"
)

func NewNSFW(s string) (NSFW, error) {
	switch NSFW(s) {
	case NSFWBlur, NSFWHide, NSFWShow:
		return NSFW(s), nil
	default:
		return "", ErrInvalidNSFW
	}
}

func (n NSFW) String() string {
	return string(n)
}

// PostView is how posts are laid out ("Вид постов" of openvk): the old
// way, the default, or as a microblog.
type PostView string

const (
	PostViewOld       PostView = "old"
	PostViewMicroblog PostView = "microblog"
)

func NewPostView(s string) (PostView, error) {
	switch PostView(s) {
	case PostViewOld, PostViewMicroblog:
		return PostView(s), nil
	default:
		return "", ErrInvalidPostView
	}
}

func (v PostView) String() string {
	return string(v)
}

// The clocks of the world run from UTC-12 to UTC+14, in steps of a
// quarter of an hour.
const (
	minTimezoneOffset  = -12 * 60
	maxTimezoneOffset  = 14 * 60
	timezoneOffsetStep = 15
)

// Timezone is the clock the owner reads dates by ("Часовой пояс" of
// openvk): the browser's, the zero value, or a fixed one.
type Timezone struct {
	fixed   bool
	minutes int
}

// NewTimezone is a fixed clock, minutes east of UTC.
func NewTimezone(minutes int) (Timezone, error) {
	if minutes < minTimezoneOffset || minutes > maxTimezoneOffset || minutes%timezoneOffsetStep != 0 {
		return Timezone{}, ErrInvalidTimezone
	}

	return Timezone{fixed: true, minutes: minutes}, nil
}

// Offset is the minutes east of UTC of a fixed clock; false means the
// browser's own.
func (t Timezone) Offset() (int, bool) {
	return t.minutes, t.fixed
}

// Interface is how the site looks to its owner ("Интерфейс" of openvk's
// settings): nobody else sees or is touched by it.
type Interface struct {
	AvatarStyle AvatarStyle
	Theme       Theme
	// HideRating takes "Заполненность страницы" off the pages the owner
	// reads ("Рейтинг" of openvk).
	HideRating bool
	NSFW       NSFW
	PostView   PostView
	MainPage   MainPage
	Timezone   Timezone
	// HiddenMenu are the links taken out of the left menu, each once, in
	// the order of the menu.
	HiddenMenu []MenuItem
}

// DefaultInterface is the site as a new user sees it.
func DefaultInterface() Interface {
	return Interface{
		AvatarStyle: AvatarsSquare,
		Theme:       ThemeDefault,
		NSFW:        NSFWBlur,
		PostView:    PostViewOld,
		MainPage:    MainPagePage,
	}
}

// InterfaceForm is the settings form as it came: NewInterface checks it.
type InterfaceForm struct {
	AvatarStyle string
	Theme       string
	HideRating  bool
	NSFW        string
	PostView    string
	MainPage    string
	// Timezone is minutes east of UTC, nil for the browser's clock.
	Timezone   *int
	HiddenMenu []string
}

// NewInterface reads the settings form: every value is one of those the
// site knows, and a link hidden twice is hidden once.
func NewInterface(form InterfaceForm) (Interface, error) {
	style, err := NewAvatarStyle(form.AvatarStyle)
	if err != nil {
		return Interface{}, err
	}

	theme, err := NewTheme(form.Theme)
	if err != nil {
		return Interface{}, err
	}

	nsfw, err := NewNSFW(form.NSFW)
	if err != nil {
		return Interface{}, err
	}

	view, err := NewPostView(form.PostView)
	if err != nil {
		return Interface{}, err
	}

	main, err := NewMainPage(form.MainPage)
	if err != nil {
		return Interface{}, err
	}

	var zone Timezone
	if form.Timezone != nil {
		if zone, err = NewTimezone(*form.Timezone); err != nil {
			return Interface{}, err
		}
	}

	hidden, err := newHiddenMenu(form.HiddenMenu)
	if err != nil {
		return Interface{}, err
	}

	return Interface{
		AvatarStyle: style,
		Theme:       theme,
		HideRating:  form.HideRating,
		NSFW:        nsfw,
		PostView:    view,
		MainPage:    main,
		Timezone:    zone,
		HiddenMenu:  hidden,
	}, nil
}

func newHiddenMenu(items []string) ([]MenuItem, error) {
	for _, item := range items {
		if !slices.Contains(menuItems, MenuItem(item)) {
			return nil, ErrInvalidMenuItem
		}
	}

	var hidden []MenuItem
	for _, item := range menuItems {
		if slices.Contains(items, item.String()) {
			hidden = append(hidden, item)
		}
	}

	return hidden, nil
}

// Interface is how the site looks to the user.
func (u *User) Interface() Interface {
	return u.ui
}

// UpdateInterface replaces the settings of "Интерфейс".
func (u *User) UpdateInterface(ui Interface) {
	u.ui = ui
}
