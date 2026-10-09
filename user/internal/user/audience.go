package user

import (
	"errors"

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
// pronouns, which speak of the person), the albums, the follows and the
// communities.
type Access struct {
	Page    bool
	Info    bool
	Photos  bool
	Friends bool
	Clubs   bool
}

// AccessFor is what viewerID (uuid.Nil: anonymous), tied to u by t, may
// see of u's page. The owner sees everything; of a deleted page there is
// nothing but the page saying so. A block closes contact, not what the
// page shows, so it does not hide any of it.
func (u *User) AccessFor(viewerID uuid.UUID, t Ties) Access {
	if viewerID == u.id {
		return Access{Page: true, Info: true, Photos: true, Friends: true, Clubs: true}
	}

	if u.IsDeactivated() {
		return Access{Page: true}
	}

	if !u.privacy.PageRead.admits(viewerID, t) {
		return Access{}
	}

	return Access{
		Page:    true,
		Info:    u.privacy.InfoRead.admits(viewerID, t),
		Photos:  u.privacy.PhotosRead.admits(viewerID, t),
		Friends: u.privacy.FriendsRead.admits(viewerID, t),
		Clubs:   Audience(u.privacy.ClubsRead).admits(viewerID, t),
	}
}
