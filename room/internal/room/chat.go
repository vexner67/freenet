package room

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxChatLen = 1000

type ChatText string

func NewChatText(s string) (ChatText, error) {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxChatLen {
		return "", ErrInvalidChat
	}
	return ChatText(s), nil
}
func (t ChatText) String() string { return string(t) }

type Chat struct {
	id, roomID, authorID, stickerID uuid.UUID
	text                            ChatText
	createdAt                       time.Time
}

func NewChat(roomID, authorID, stickerID uuid.UUID, text ChatText, now time.Time) (Chat, error) {
	if roomID == uuid.Nil || authorID == uuid.Nil {
		return Chat{}, ErrInvalidChat
	}
	if stickerID == uuid.Nil {
		normalized, err := NewChatText(text.String())
		if err != nil {
			return Chat{}, err
		}
		text = normalized
	} else if text != "" {
		return Chat{}, ErrInvalidChat
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Chat{}, fmt.Errorf("uuid.NewV7: %w", err)
	}
	return RestoreChat(id, roomID, authorID, stickerID, text, now), nil
}
func RestoreChat(id, roomID, authorID, stickerID uuid.UUID, text ChatText, now time.Time) Chat {
	return Chat{id: id, roomID: roomID, authorID: authorID, stickerID: stickerID, text: text, createdAt: now.UTC()}
}
func (c Chat) ID() uuid.UUID        { return c.id }
func (c Chat) RoomID() uuid.UUID    { return c.roomID }
func (c Chat) AuthorID() uuid.UUID  { return c.authorID }
func (c Chat) StickerID() uuid.UUID { return c.stickerID }
func (c Chat) Text() ChatText       { return c.text }
func (c Chat) CreatedAt() time.Time { return c.createdAt }
