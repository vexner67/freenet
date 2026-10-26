package user

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxNicknameLen = 32

var (
	ErrNicknameEmpty    = errors.New("nickname is empty")
	ErrNicknameTooLong  = errors.New("nickname is too long")
	ErrNicknameInvalid  = errors.New("nickname contains invalid characters")
	ErrNicknameReserved = errors.New("nickname is reserved")

	nicknamePattern = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z0-9_]*[A-Za-z0-9])?$`)

	// Profiles live at /<nickname> on the website, so a nickname must not
	// take the path of a page or of the API.
	reservedNicknames = map[string]struct{}{
		"about": {}, "admin": {}, "album": {}, "albums": {}, "api": {}, "business": {}, "casino": {}, "club": {}, "docs": {}, "edit": {}, "events": {}, "fave": {}, "feed": {},
		"follows": {}, "freenet": {}, "gift": {}, "gifts": {}, "groups": {}, "guests": {}, "healthz": {}, "help": {}, "id": {}, "im": {}, "invite": {},
		"language": {}, "login": {}, "logout": {}, "market": {}, "me": {}, "media": {}, "mine": {}, "news": {}, "note": {}, "notes": {},
		"notifications": {}, "people": {}, "pet": {}, "photo": {}, "post": {}, "prime": {}, "search": {},
		"settings": {}, "shop": {}, "static": {}, "stickers": {}, "support": {}, "terms": {}, "top": {}, "topic": {}, "votes": {}, "wall": {},
	}
)

type Nickname string

func NewNickname(s string) (Nickname, error) {
	if s == "" {
		return "", ErrNicknameEmpty
	}

	if utf8.RuneCountInString(s) > maxNicknameLen {
		return "", ErrNicknameTooLong
	}

	if !nicknamePattern.MatchString(s) {
		return "", ErrNicknameInvalid
	}

	if _, ok := reservedNicknames[strings.ToLower(s)]; ok {
		return "", ErrNicknameReserved
	}

	return Nickname(s), nil
}

func (n Nickname) String() string {
	return string(n)
}
