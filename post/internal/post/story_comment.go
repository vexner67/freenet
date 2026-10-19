package post

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxStoryCommentLength bounds a comment under a story: it is read in the
// viewer, over the photo, and lists carry it whole.
const MaxStoryCommentLength = 1000

var (
	ErrStoryCommentTooLong      = errors.New("story comment is too long")
	ErrStoryCommentNotFound     = errors.New("story comment not found")
	ErrCannotDeleteStoryComment = errors.New("only the author of the comment or of the story can delete it")
)

// StoryComment is a comment under a story, which those who see the story
// read under its photo while it is shown: text only, gone with the story.
type StoryComment struct {
	id        uuid.UUID
	storyID   uuid.UUID
	authorID  uuid.UUID
	text      CommentText
	createdAt time.Time
}

// NewStoryComment comments storyID; the comments of a story are ordered
// and paged by ID, the oldest first.
func NewStoryComment(storyID, authorID uuid.UUID, text CommentText, now time.Time) (*StoryComment, error) {
	if text == "" {
		return nil, ErrCommentEmpty
	}

	if utf8.RuneCountInString(text.String()) > MaxStoryCommentLength {
		return nil, ErrStoryCommentTooLong
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &StoryComment{
		id: id, storyID: storyID, authorID: authorID, text: text,
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredStoryComment is a comment as storage keeps it.
type StoredStoryComment struct {
	ID        uuid.UUID
	StoryID   uuid.UUID
	AuthorID  uuid.UUID
	Text      CommentText
	CreatedAt time.Time
}

// RestoreStoryComment rebuilds a comment from storage without validation.
func RestoreStoryComment(s StoredStoryComment) *StoryComment {
	return &StoryComment{id: s.ID, storyID: s.StoryID, authorID: s.AuthorID, text: s.Text, createdAt: s.CreatedAt.UTC()}
}

func (c *StoryComment) ID() uuid.UUID {
	return c.id
}

func (c *StoryComment) StoryID() uuid.UUID {
	return c.storyID
}

func (c *StoryComment) AuthorID() uuid.UUID {
	return c.authorID
}

func (c *StoryComment) Text() CommentText {
	return c.text
}

func (c *StoryComment) CreatedAt() time.Time {
	return c.createdAt
}

// CheckDelete: its author or the author of the story deletes a comment.
func (c *StoryComment) CheckDelete(requesterID uuid.UUID, story *Story) error {
	if requesterID != c.authorID && !story.Mine(requesterID) {
		return ErrCannotDeleteStoryComment
	}

	return nil
}

// Notifies tells whether the author of the story hears of what actorID
// did with it, a like or a comment: not of their own.
func (s *Story) Notifies(actorID uuid.UUID) bool {
	return !s.Mine(actorID)
}
