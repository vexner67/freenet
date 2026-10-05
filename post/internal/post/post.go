// Package post is the domain of wall posts.
package post

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxTextLen = 1000

var (
	ErrTextEmpty   = errors.New("post text is empty")
	ErrTextTooLong = errors.New("post text is too long")
	ErrNotFound    = errors.New("post not found")
	ErrNotAuthor   = errors.New("only the author can delete the post")
)

// Text is what a post says: trimmed, with "\n" line breaks.
type Text string

func NewText(s string) (Text, error) {
	// Normalise line breaks so length and storage do not depend on the client.
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if s == "" {
		return "", ErrTextEmpty
	}

	if utf8.RuneCountInString(s) > maxTextLen {
		return "", ErrTextTooLong
	}

	return Text(s), nil
}

func (t Text) String() string {
	return string(t)
}

type Post struct {
	id        uuid.UUID
	authorID  uuid.UUID
	text      Text
	createdAt time.Time
}

// New creates a post. Its UUIDv7 ID grows with time, so walls are ordered
// and paged by ID.
func New(authorID uuid.UUID, text Text, now time.Time) (*Post, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Post{
		id:       id,
		authorID: authorID,
		text:     text,
		// PostgreSQL keeps microseconds; trim now so a post reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Restore rebuilds a post from storage without validation or new IDs.
func Restore(id, authorID uuid.UUID, text Text, createdAt time.Time) *Post {
	return &Post{
		id:        id,
		authorID:  authorID,
		text:      text,
		createdAt: createdAt,
	}
}

// CanBeDeletedBy: a wall post is removed only by whoever wrote it.
func (p *Post) CanBeDeletedBy(userID uuid.UUID) bool {
	return p.authorID == userID
}

func (p *Post) ID() uuid.UUID {
	return p.id
}

func (p *Post) AuthorID() uuid.UUID {
	return p.authorID
}

func (p *Post) Text() Text {
	return p.text
}

func (p *Post) CreatedAt() time.Time {
	return p.createdAt
}
