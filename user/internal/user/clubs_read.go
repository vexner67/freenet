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
	ClubsReadFollowing ClubsRead = "following"
	ClubsReadOnlyMe    ClubsRead = "only_me"
)

// DefaultClubsRead shows the communities to everyone, as openvk does.
const DefaultClubsRead = ClubsReadEveryone

var ErrInvalidClubsRead = errors.New("invalid clubs read setting")

func NewClubsRead(s string) (ClubsRead, error) {
	switch ClubsRead(s) {
	case ClubsReadEveryone, ClubsReadFollowing, ClubsReadOnlyMe:
		return ClubsRead(s), nil
	default:
		return "", ErrInvalidClubsRead
	}
}

func (r ClubsRead) String() string {
	return string(r)
}

// ClubsVisibleTo tells whether viewerID (uuid.Nil: anonymous) may see
// the communities u is in. The owner always may; nobody sees those of a
// deleted page. A block closes contact, not what the page shows, so it
// does not hide them.
func (u *User) ClubsVisibleTo(viewerID uuid.UUID, t Ties) bool {
	if viewerID == u.id {
		return true
	}

	if u.IsDeactivated() {
		return false
	}

	switch u.privacy.ClubsRead {
	case ClubsReadEveryone:
		return true
	case ClubsReadFollowing:
		return t.Followed
	default:
		return false
	}
}
