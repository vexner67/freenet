package user

import (
	"errors"

	"github.com/google/uuid"
)

// MessagesWrite is who may write private messages to a user, as
// "Кто может писать мне сообщения" on openvk. As with the wall, openvk's
// "friends" are the people the user follows.
type MessagesWrite string

const (
	MessagesWriteEveryone  MessagesWrite = "everyone"
	MessagesWriteFollowing MessagesWrite = "following"
	MessagesWriteNobody    MessagesWrite = "nobody"
)

// DefaultMessagesWrite keeps messages open, as they were before the setting.
const DefaultMessagesWrite = MessagesWriteEveryone

var ErrInvalidMessagesWrite = errors.New("invalid messages write setting")

func NewMessagesWrite(s string) (MessagesWrite, error) {
	switch MessagesWrite(s) {
	case MessagesWriteEveryone, MessagesWriteFollowing, MessagesWriteNobody:
		return MessagesWrite(s), nil
	default:
		return "", ErrInvalidMessagesWrite
	}
}

func (w MessagesWrite) String() string {
	return string(w)
}

// Ties are how another user is tied to u, for what u lets them do.
type Ties struct {
	// Followed: u follows them.
	Followed bool
	// Blocked: either of the two has blacklisted the other.
	Blocked bool
	// WroteFirst: u has written to them in private messages.
	WroteFirst bool
}

// CanBeMessagedBy tells whether senderID may write to u. Nobody writes to
// a deleted page, and a block shuts messages both ways; otherwise u's
// setting decides, but whoever u wrote to may answer, as openvk promises:
// "Вам смогут ответить, если вы напишете первыми".
func (u *User) CanBeMessagedBy(senderID uuid.UUID, t Ties) bool {
	if senderID == u.id || t.Blocked || u.IsDeactivated() {
		return false
	}

	if t.WroteFirst {
		return true
	}

	switch u.privacy.MessagesWrite {
	case MessagesWriteEveryone:
		return true
	case MessagesWriteFollowing:
		return t.Followed
	default:
		return false
	}
}
