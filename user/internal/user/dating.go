package user

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// DatingGoal is what a person looks for in "Знакомства".
type DatingGoal string

const (
	DatingFriendship DatingGoal = "friendship"
	DatingChat       DatingGoal = "chat"
	DatingLove       DatingGoal = "love"
)

// MaxDatingAbout is the longest text a dating card tells about its owner.
const MaxDatingAbout = 300

// DatingDeckSize is how many cards "Знакомства" show at once.
const DatingDeckSize = 10

var (
	ErrDatingGoal  = errors.New("there is no such goal of dating")
	ErrDatingSeek  = errors.New("there is no such gender to seek")
	ErrDatingAbout = errors.New("the text about yourself is too long")
	ErrDatingOff   = errors.New("dating is off")
	ErrDatingSelf  = errors.New("you cannot like yourself")
	ErrDatingTwice = errors.New("you have already answered this card")
	// ErrDatingProfile: without a nickname a person has no name to show.
	ErrDatingProfile = errors.New("fill in the profile before dating")
)

func NewDatingGoal(s string) (DatingGoal, error) {
	switch g := DatingGoal(s); g {
	case DatingFriendship, DatingChat, DatingLove:
		return g, nil
	default:
		return "", ErrDatingGoal
	}
}

// NewDatingSeek is the gender a person wants to meet: GenderNone for
// anyone, a man or a woman.
func NewDatingSeek(s string) (Gender, error) {
	switch g := Gender(s); g {
	case GenderNone, GenderMale, GenderFemale:
		return g, nil
	default:
		return "", ErrDatingSeek
	}
}

// DatingCard is how a person shows themselves in "Знакомства". A card
// that is off is not shown and shows nothing.
type DatingCard struct {
	UserID    uuid.UUID
	Goal      DatingGoal
	Seek      Gender
	About     string
	On        bool
	UpdatedAt time.Time
}

// NewDatingCard checks what a person tells; the about text is trimmed.
func NewDatingCard(userID uuid.UUID, goal DatingGoal, seek Gender, about string, on bool, now time.Time) (DatingCard, error) {
	about = strings.TrimSpace(about)
	if utf8.RuneCountInString(about) > MaxDatingAbout {
		return DatingCard{}, ErrDatingAbout
	}

	return DatingCard{UserID: userID, Goal: goal, Seek: seek, About: about, On: on, UpdatedAt: now.UTC()}, nil
}

// Accepts tells whether the card wants to meet a person of gender g: a
// card seeking anyone accepts everybody.
func (c DatingCard) Accepts(g Gender) bool {
	return c.Seek == GenderNone || c.Seek == g
}

// Suits tells whether other, of gender theirs, is shown to the owner of
// c, of gender mine: both cards are on and each accepts the other.
func (c DatingCard) Suits(mine Gender, other DatingCard, theirs Gender) bool {
	return c.On && other.On && c.UserID != other.UserID && c.Accepts(theirs) && other.Accepts(mine)
}

// DatingMatch is a person who liked the owner back, and when the second of
// the two likes came.
type DatingMatch struct {
	UserID uuid.UUID
	At     time.Time
}

func (g DatingGoal) String() string {
	return string(g)
}
