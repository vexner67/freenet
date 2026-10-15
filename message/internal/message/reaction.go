package message

import (
	"errors"

	"github.com/google/uuid"
)

var ErrInvalidReaction = errors.New("invalid reaction")

// Reaction is how someone answers a message without words, as on VK: one
// of a few emoji under it. The page draws them; the name is what is kept.
type Reaction string

const (
	ReactionHeart Reaction = "heart"
	ReactionLike  Reaction = "like"
	ReactionLaugh Reaction = "laugh"
	ReactionWow   Reaction = "wow"
	ReactionSad   Reaction = "sad"
	ReactionFire  Reaction = "fire"
)

var reactions = []Reaction{ReactionHeart, ReactionLike, ReactionLaugh, ReactionWow, ReactionSad, ReactionFire}

func NewReaction(s string) (Reaction, error) {
	for _, r := range reactions {
		if string(r) == s {
			return r, nil
		}
	}

	return "", ErrInvalidReaction
}

func (r Reaction) String() string {
	return string(r)
}

// Reacted is the reaction of one person to a message: each has one at a
// time, and a new one takes its place.
type Reacted struct {
	UserID   uuid.UUID
	Reaction Reaction
}

// Reactions are those of a message, the oldest first.
type Reactions []Reacted

// CheckReact tells whether userID may react to m: one side that has it,
// and not once it is taken back.
func (m *Message) CheckReact(userID uuid.UUID) error {
	if !m.VisibleTo(userID) || m.IsDeleted() {
		return ErrNotFound
	}

	return nil
}

// CheckReact tells whether a member may react to m, a message of the chat
// they are in: one written by a person, they are shown and not taken
// back. Who has left reacts no more.
func (m *ChatMessage) CheckReact(me Member) error {
	if err := me.CheckActive(); err != nil {
		return err
	}

	if m.IsAction() || m.IsDeleted() {
		return ErrNotFound
	}

	return nil
}

// Reactions are those of m, as read with it; a message taken back has
// none.
func (m *Message) Reactions() Reactions {
	if m.IsDeleted() {
		return nil
	}

	return m.reactions
}

// Reacting is m with reactions r.
func (m *Message) Reacting(r Reactions) *Message {
	reacted := *m
	reacted.reactions = r

	return &reacted
}

// Reactions are those of m, as Message.Reactions.
func (m *ChatMessage) Reactions() Reactions {
	if m.IsDeleted() {
		return nil
	}

	return m.reactions
}

// Reacting is m with reactions r.
func (m *ChatMessage) Reacting(r Reactions) *ChatMessage {
	reacted := *m
	reacted.reactions = r

	return &reacted
}
