package user

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxWebsiteLen = 256
	maxAddressLen = 255
)

var (
	ErrTelegramInvalid = errors.New("invalid telegram username")
	ErrWebsiteTooLong  = errors.New("website is too long")
	ErrWebsiteInvalid  = errors.New("website is not a link")
	ErrAddressTooLong  = errors.New("address is too long")
)

// Contacts are openvk's "Контактная информация": everything optional.
type Contacts struct {
	Email    Email
	Telegram Telegram
	Website  Website
	City     Place
	Address  Address
}

// NewContactEmail is an email to show on the page, empty for none; it
// need not be the one to sign in with.
func NewContactEmail(s string) (Email, error) {
	s, err := visibleText(s)
	if err != nil || s == "" {
		return "", err
	}

	return NewEmail(s)
}

// telegramUsername is Telegram's rule for usernames.
var telegramUsername = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

// Telegram is a username without "@", as openvk keeps it; "@name" and
// t.me links are taken too.
type Telegram string

func NewTelegram(s string) (Telegram, error) {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"https://t.me/", "http://t.me/", "t.me/", "@"} {
		s = strings.TrimPrefix(s, prefix)
	}

	if s == "" {
		return "", nil
	}

	if !telegramUsername.MatchString(s) {
		return "", ErrTelegramInvalid
	}

	return Telegram(s), nil
}

func (t Telegram) String() string {
	return string(t)
}

// Website is "Личный сайт": an http or https link; https:// is added when
// no scheme is given, as openvk does.
type Website string

func NewWebsite(s string) (Website, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	if s == "" {
		return "", nil
	}

	if !strings.Contains(s, "://") {
		s = "https://" + s
	}

	if utf8.RuneCountInString(s) > maxWebsiteLen {
		return "", ErrWebsiteTooLong
	}

	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(s, " \t\n") {
		return "", ErrWebsiteInvalid
	}

	return Website(s), nil
}

func (w Website) String() string {
	return string(w)
}

// Address is "Адрес": one line, trimmed.
type Address string

func NewAddress(s string) (Address, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) > maxAddressLen {
		return "", ErrAddressTooLong
	}

	return Address(s), nil
}

func (a Address) String() string {
	return string(a)
}
