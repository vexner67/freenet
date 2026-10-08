package post

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSuggestionNotFound = errors.New("suggestion not found")
	ErrCannotWithdraw     = errors.New("only the author or the admins can delete the suggestion")
	ErrCannotPublish      = errors.New("only the admins can publish the suggestion")
	ErrCannotSeeAll       = errors.New("only the admins see all the suggestions")
)

// Suggestion is a post suggested for a community's wall, openvk's
// "Предложить новость" on a "Предложка": nobody but its author and the
// community's admins sees it until an admin publishes it for the
// community, or it is withdrawn or declined. Its content is text, photos or
// both, never a repost.
type Suggestion struct {
	id        uuid.UUID
	wallID    uuid.UUID
	authorID  uuid.UUID
	content   Content
	createdAt time.Time
}

// NewSuggestion suggests content by authorID for the wall of wallID.
// Whether the author may suggest there is the community's, which the
// caller checks.
func NewSuggestion(wallID, authorID uuid.UUID, content Content, now time.Time) (*Suggestion, error) {
	if content.RepostOf() != uuid.Nil {
		return nil, ErrInvalidRepost
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Suggestion{
		id:       id,
		wallID:   wallID,
		authorID: authorID,
		content:  content,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredSuggestion is a suggestion as storage keeps it.
type StoredSuggestion struct {
	ID        uuid.UUID
	WallID    uuid.UUID
	AuthorID  uuid.UUID
	Content   Content
	CreatedAt time.Time
}

// RestoreSuggestion rebuilds a suggestion from storage without
// validation or new IDs.
func RestoreSuggestion(s StoredSuggestion) *Suggestion {
	return &Suggestion{id: s.ID, wallID: s.WallID, authorID: s.AuthorID, content: s.Content, createdAt: s.CreatedAt}
}

// Published is the post an admin makes of s at now, as openvk's
// "Опубликовать запись": it speaks for the community and, signed, names
// the author ("Подпись автора"); text is what it says, as the admin may
// have changed it, with the suggested photos. The post is new, dated now,
// so it shows on top of the wall, and the community's: its author no
// longer edits or deletes it. Whether the admin runs the community is
// the club service's, which the caller asks.
func (s *Suggestion) Published(text Text, signed bool, now time.Time) (*Post, error) {
	content, err := NewContent(text, s.content.Photos())
	if err != nil {
		return nil, err
	}

	p, err := NewFromWall(s.wallID, s.authorID, content, signed, now)
	if err != nil {
		return nil, err
	}

	p.suggested = true

	return p, nil
}

// CanBeWithdrawnBy: the author takes a suggestion back; the admins, whom
// the caller asks, decline it.
func (s *Suggestion) CanBeWithdrawnBy(userID uuid.UUID) bool {
	return userID == s.authorID
}

func (s *Suggestion) ID() uuid.UUID {
	return s.id
}

func (s *Suggestion) WallID() uuid.UUID {
	return s.wallID
}

func (s *Suggestion) AuthorID() uuid.UUID {
	return s.authorID
}

func (s *Suggestion) Content() Content {
	return s.content
}

func (s *Suggestion) CreatedAt() time.Time {
	return s.createdAt
}
