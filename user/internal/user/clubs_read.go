package user

import (
	"errors"

	"github.com/google/uuid"
)

// ClubsRead is who sees which communities a user is in, as "Кому видно
// мои группы" on openvk. The communities show their members whatever the
// setting: it hides the list on the user's page, not the membership.
type ClubsRead string

const (
	ClubsReadEveryone  ClubsRead = "everyone"
	ClubsReadUsers     ClubsRead = "users"
	ClubsReadFollowing ClubsRead = "following"
	ClubsReadOnlyMe    ClubsRead = "only_me"
)

// DefaultClubsRead shows the communities to everyone, as openvk does.
const DefaultClubsRead = ClubsReadEveryone

var ErrInvalidClubsRead = errors.New("invalid clubs read setting")

func NewClubsRead(s string) (ClubsRead, error) {
	switch ClubsRead(s) {
	case ClubsReadEveryone, ClubsReadUsers, ClubsReadFollowing, ClubsReadOnlyMe:
		return ClubsRead(s), nil
	default:
		return "", ErrInvalidClubsRead
	}
}

func (r ClubsRead) String() string {
	return string(r)
}

// ClubsVisibleTo tells whether viewerID (uuid.Nil: anonymous) may see
// the communities u is in (see AccessFor).
func (u *User) ClubsVisibleTo(viewerID uuid.UUID, t Ties) bool {
	return u.AccessFor(viewerID, t).Clubs
}
