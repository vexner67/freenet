package user

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// TitleIcon is the picture a title is shown with.
type TitleIcon string

const (
	TitleCrown     TitleIcon = "crown"
	TitleCup       TitleIcon = "cup"
	TitleStar      TitleIcon = "star"
	TitleLaurel    TitleIcon = "laurel"
	TitleShield    TitleIcon = "shield"
	TitleFlame     TitleIcon = "flame"
	TitleDiamond   TitleIcon = "diamond"
	TitleLightning TitleIcon = "lightning"
)

// TitleIcons are all pictures in the order the site offers them.
var TitleIcons = []TitleIcon{TitleCrown, TitleCup, TitleStar, TitleLaurel, TitleShield, TitleFlame, TitleDiamond, TitleLightning}

// MaxTitleNameLength is in characters.
const MaxTitleNameLength = 40

var (
	ErrUnknownTitleIcon = errors.New("unknown title icon")
	ErrTitleNameEmpty   = errors.New("the name of a title is empty")
	ErrTitleNameTooLong = errors.New("the name of a title is too long")
	ErrTitleNameLines   = errors.New("the name of a title is one line")
	ErrTitleNotFound    = errors.New("title not found")
)

func NewTitleIcon(s string) (TitleIcon, error) {
	for _, icon := range TitleIcons {
		if string(icon) == s {
			return icon, nil
		}
	}

	return "", ErrUnknownTitleIcon
}

func (i TitleIcon) String() string {
	return string(i)
}

// TitleName is what a title calls its holder: "Лучший игрок недели".
type TitleName string

func NewTitleName(s string) (TitleName, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	switch {
	case s == "":
		return "", ErrTitleNameEmpty
	case strings.ContainsAny(s, "\r\n"):
		return "", ErrTitleNameLines
	case utf8.RuneCountInString(s) > MaxTitleNameLength:
		return "", ErrTitleNameTooLong
	}

	return TitleName(s), nil
}

func (n TitleName) String() string {
	return string(n)
}

// Title is an award the administrators give a page: a name and a picture.
// A page may hold the same title any number of times.
type Title struct {
	id        uuid.UUID
	userID    uuid.UUID
	name      TitleName
	icon      TitleIcon
	grantedAt time.Time
}

// NewTitle is the title name with icon given to userID at now.
func NewTitle(userID uuid.UUID, name TitleName, icon TitleIcon, now time.Time) (*Title, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return RestoreTitle(id, userID, name, icon, now), nil
}

// RestoreTitle is a title as the storage keeps it.
func RestoreTitle(id, userID uuid.UUID, name TitleName, icon TitleIcon, grantedAt time.Time) *Title {
	return &Title{id: id, userID: userID, name: name, icon: icon, grantedAt: grantedAt.UTC()}
}

func (t *Title) ID() uuid.UUID {
	return t.id
}

func (t *Title) UserID() uuid.UUID {
	return t.userID
}

func (t *Title) Name() TitleName {
	return t.name
}

func (t *Title) Icon() TitleIcon {
	return t.icon
}

func (t *Title) GrantedAt() time.Time {
	return t.grantedAt
}
