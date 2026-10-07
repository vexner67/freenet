package user

import (
	"errors"

	"github.com/google/uuid"
)

// WallWrite is who may write on a user's wall besides the user, as
// "Кто может писать у меня на стене" on openvk. There are no friends here:
// openvk's "friends" are the people the user follows.
type WallWrite string

const (
	WallWriteEveryone  WallWrite = "everyone"
	WallWriteFollowing WallWrite = "following"
	WallWriteOnlyMe    WallWrite = "only_me"
)

// DefaultWallWrite keeps strangers off a new wall until the owner opens it.
const DefaultWallWrite = WallWriteFollowing

var ErrInvalidWallWrite = errors.New("invalid wall write setting")

func NewWallWrite(s string) (WallWrite, error) {
	switch WallWrite(s) {
	case WallWriteEveryone, WallWriteFollowing, WallWriteOnlyMe:
		return WallWrite(s), nil
	default:
		return "", ErrInvalidWallWrite
	}
}

func (w WallWrite) String() string {
	return string(w)
}

// CanPostOnWall tells whether authorID may write on u's wall. The owner
// always may; a block shuts the wall both ways.
func (u *User) CanPostOnWall(authorID uuid.UUID, t Ties) bool {
	if authorID == u.id {
		return true
	}

	if t.Blocked {
		return false
	}

	switch u.privacy.WallWrite {
	case WallWriteEveryone:
		return true
	case WallWriteFollowing:
		return t.Followed
	default:
		return false
	}
}
