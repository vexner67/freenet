package club

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

const maxWebsiteLen = 256

var (
	ErrWebsiteTooLong = errors.New("club website is too long")
	ErrWebsiteInvalid = errors.New("club website is not a link")
)

// Website is the community's site, "Сайт" of openvk: an http or https link,
// https:// added when the scheme is left out, as openvk does; "" is none.
type Website string

func NewWebsite(s string) (Website, error) {
	s = strings.TrimSpace(s)
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
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		strings.ContainsAny(s, " \t\n\r") {
		return "", ErrWebsiteInvalid
	}

	return Website(s), nil
}

func (w Website) String() string {
	return string(w)
}
