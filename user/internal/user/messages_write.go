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
	// Prime: they have Freenet Prime now.
	Prime bool
}

// CanBeMessagedBy tells whether senderID may write to u. Nobody writes to
// a deleted page, and a block shuts messages both ways; otherwise u's
// setting decides, but whoever u wrote to may answer, as openvk promises:
// "Вам смогут ответить, если вы напишете первыми". Everyone writes to
// themselves: that dialog is «Избранное».
func (u *User) CanBeMessagedBy(senderID uuid.UUID, t Ties) bool {
	if senderID == u.id {
		return !u.IsDeactivated()
	}

	if t.Blocked || u.IsDeactivated() {
		return false
	}

	if t.WroteFirst {
		return true
	}

	if u.privacy.MessagesPrime && !t.Prime {
		return false
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

// MaxMessagePrice is the most votes a message to a user may cost.
const MaxMessagePrice = 1_000_000

var ErrInvalidMessagePrice = errors.New("the price of a message must be between 0 and 1000000 votes")

// MessagePrice is how many votes each message to a user costs whoever
// writes ("Сообщения за голоса"); 0 for free. The votes go to the user.
type MessagePrice int64

func NewMessagePrice(votes int64) (MessagePrice, error) {
	if votes < 0 || votes > MaxMessagePrice {
		return 0, ErrInvalidMessagePrice
	}

	return MessagePrice(votes), nil
}

func (p MessagePrice) Int64() int64 {
	return int64(p)
}

// WantsPrimeFrom tells whether u refuses senderID only for having no
// Freenet Prime, so that the sender may be told so.
func (u *User) WantsPrimeFrom(senderID uuid.UUID, t Ties) bool {
	return senderID != u.id && u.privacy.MessagesPrime && !t.Prime && !t.WroteFirst
}

// MessagePriceFor is what a message of senderID to u costs: nothing to
// themselves («Избранное») and to whom u wrote first — that is an
// answer, as with the setting of who may write.
func (u *User) MessagePriceFor(senderID uuid.UUID, t Ties) int64 {
	if senderID == u.id || t.WroteFirst {
		return 0
	}

	return u.privacy.MessagePrice.Int64()
}
