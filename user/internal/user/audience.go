package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Audience is who sees a part of a page, as openvk's "Кому видно …":
// anyone, signed-in people ("Пользователям"), the people the owner
// follows (openvk's "Друзья") or only the owner.
type Audience string

const (
	AudienceEveryone  Audience = "everyone"
	AudienceUsers     Audience = "users"
	AudienceFollowing Audience = "following"
	AudienceOnlyMe    Audience = "only_me"
)

var (
	ErrInvalidAudience = errors.New("invalid audience")
	// ErrInvalidPageRead: the page itself is for anyone or for users only,
	// as openvk's "Кому в интернете видно мою страницу".
	ErrInvalidPageRead = errors.New("invalid page read setting")
)

func NewAudience(s string) (Audience, error) {
	switch Audience(s) {
	case AudienceEveryone, AudienceUsers, AudienceFollowing, AudienceOnlyMe:
		return Audience(s), nil
	default:
		return "", ErrInvalidAudience
	}
}

// NewPageRead is the audience of the page itself: everyone or users.
func NewPageRead(s string) (Audience, error) {
	switch Audience(s) {
	case AudienceEveryone, AudienceUsers:
		return Audience(s), nil
	default:
		return "", ErrInvalidPageRead
	}
}

func (a Audience) String() string {
	return string(a)
}

// admits tells whether a viewer (uuid.Nil: anonymous), tied to the owner
// by t, is in the audience; the owner is decided by the caller.
func (a Audience) admits(viewerID uuid.UUID, t Ties) bool {
	switch a {
	case AudienceEveryone:
		return true
	case AudienceUsers:
		return viewerID != uuid.Nil
	case AudienceFollowing:
		return t.Followed
	default:
		return false
	}
}

// Access is what of a page a viewer may see: the page itself, its
// information (everything but the names, the photo, the status and the
// pronouns, which speak of the person), the albums, the follows, the
// communities, the notes and the gifts.
type Access struct {
	Page    bool
	Info    bool
	Photos  bool
	Friends bool
	Clubs   bool
	Notes   bool
	Gifts   bool
}

// AccessFor is what viewerID (uuid.Nil: anonymous), tied to u by t, may
// see of u's page at now. The owner sees everything; of a deleted page
// and of one blocked at now there is nothing but the page saying so. A block closes contact, not what the
// page shows, so it does not hide any of it. A closed page ("Тип
// профиля: закрытый") shows itself only to those its owner follows.
func (u *User) AccessFor(viewerID uuid.UUID, t Ties, now time.Time) Access {
	return u.access(viewerID, t, u.IsBannedAt(now))
}

// access is AccessFor of a page banned or not.
func (u *User) access(viewerID uuid.UUID, t Ties, banned bool) Access {
	if viewerID == u.id {
		return Access{Page: true, Info: true, Photos: true, Friends: true, Clubs: true, Notes: true, Gifts: true}
	}

	// Of a deleted or a blocked page only the page itself is shown: what
	// became of it.
	if u.IsDeactivated() || banned {
		return Access{Page: true}
	}

	// A closed page is for the people its owner follows, openvk's friends.
	if !u.privacy.PageRead.admits(viewerID, t) || (u.privacy.Closed && !t.Followed) {
		return Access{}
	}

	return Access{
		Page:    true,
		Info:    u.privacy.InfoRead.admits(viewerID, t),
		Photos:  u.privacy.PhotosRead.admits(viewerID, t),
		Friends: u.privacy.FriendsRead.admits(viewerID, t),
		Clubs:   Audience(u.privacy.ClubsRead).admits(viewerID, t),
		Notes:   u.privacy.NotesRead.admits(viewerID, t),
		Gifts:   u.privacy.GiftsRead.admits(viewerID, t),
	}
}
