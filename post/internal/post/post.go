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
	ErrTextEmpty    = errors.New("post text is empty")
	ErrTextTooLong  = errors.New("post text is too long")
	ErrNotFound     = errors.New("post not found")
	ErrCannotDelete = errors.New("only the author or the owner of the wall can delete the post")
)

// Text is what a post says: trimmed, with "\n" line breaks. It may be
// empty when the post has photos (see NewContent).
type Text string

func NewText(s string) (Text, error) {
	s, err := cleanText(s, maxTextLen, nil, ErrTextTooLong)
	if err != nil {
		return "", err
	}

	return Text(s), nil
}

// cleanText trims s and normalises its line breaks, so length and storage
// do not depend on the client. A nil errEmpty allows empty text.
func cleanText(s string, maxLen int, errEmpty, errTooLong error) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if s == "" && errEmpty != nil {
		return "", errEmpty
	}

	if utf8.RuneCountInString(s) > maxLen {
		return "", errTooLong
	}

	return s, nil
}

func (t Text) String() string {
	return string(t)
}

// Counters are how many likes and comments a post has. Storage keeps them
// with the post, so a wall is read without counting.
type Counters struct {
	Likes    int
	Comments int
}

// Post is written by its author on a wall; the wall is that of its owner,
// the author's own or someone else's.
type Post struct {
	id        uuid.UUID
	wallID    uuid.UUID
	authorID  uuid.UUID
	content   Content
	createdAt time.Time
	counters  Counters
}

// New creates a post by authorID on the wall of wallID. Its UUIDv7 ID
// grows with time, so walls are ordered and paged by ID. Whether the author
// may write on that wall is the wall owner's privacy, which the caller checks.
func New(wallID, authorID uuid.UUID, content Content, now time.Time) (*Post, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Post{
		id:       id,
		wallID:   wallID,
		authorID: authorID,
		content:  content,
		// PostgreSQL keeps microseconds; trim now so a post reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Restore rebuilds a post from storage without validation or new IDs.
func Restore(id, wallID, authorID uuid.UUID, content Content, createdAt time.Time, counters Counters) *Post {
	return &Post{
		id:        id,
		wallID:    wallID,
		authorID:  authorID,
		content:   content,
		createdAt: createdAt,
		counters:  counters,
	}
}

// CanBeDeletedBy: a post is removed by whoever wrote it or by the owner of
// the wall, who keeps the wall clean.
func (p *Post) CanBeDeletedBy(userID uuid.UUID) bool {
	return userID == p.authorID || userID == p.wallID
}

// OnOwnWall tells whether the author wrote on their own wall.
func (p *Post) OnOwnWall() bool {
	return p.wallID == p.authorID
}

func (p *Post) ID() uuid.UUID {
	return p.id
}

func (p *Post) WallID() uuid.UUID {
	return p.wallID
}

func (p *Post) AuthorID() uuid.UUID {
	return p.authorID
}

func (p *Post) Content() Content {
	return p.content
}

func (p *Post) CreatedAt() time.Time {
	return p.createdAt
}

func (p *Post) Counters() Counters {
	return p.counters
}
