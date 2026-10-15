package post

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	// StoryLifetime is how long a story is shown: a day, as on VK.
	StoryLifetime = 24 * time.Hour
	// MaxStories is how many stories of one person are shown at once.
	MaxStories = 30
	// maxStoryTextLen bounds the words over the photo of a story.
	maxStoryTextLen = 200
)

var (
	ErrStoryNotFound     = errors.New("story not found")
	ErrStoryTextTooLong  = errors.New("story text is too long")
	ErrStoryWithoutPhoto = errors.New("a story shows a photo")
	ErrTooManyStories    = errors.New("too many stories")
	ErrCannotDeleteStory = errors.New("only the author can delete the story")
	ErrNotStoryAuthor    = errors.New("only the author sees who viewed the story")
)

// StoryText is what a story says over its photo: short, possibly empty.
type StoryText string

func NewStoryText(s string) (StoryText, error) {
	s, err := cleanText(s, maxStoryTextLen, nil, ErrStoryTextTooLong)
	if err != nil {
		return "", err
	}

	return StoryText(s), nil
}

func (t StoryText) String() string {
	return string(t)
}

// Story is a photo a person shows for a day, VK's "история" (openvk has
// none): it is shown to those who read the person, at the top of their
// news, and is gone after StoryLifetime. Its author alone sees who viewed
// it.
type Story struct {
	id        uuid.UUID
	authorID  uuid.UUID
	photoID   uuid.UUID
	text      StoryText
	createdAt time.Time
}

// NewStory is a story authorID tells at now with a photo of theirs: the
// caller has checked that the photo is theirs.
func NewStory(authorID, photoID uuid.UUID, text StoryText, now time.Time) (*Story, error) {
	if photoID == uuid.Nil {
		return nil, ErrStoryWithoutPhoto
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Story{id: id, authorID: authorID, photoID: photoID, text: text, createdAt: now.UTC()}, nil
}

// RestoreStory builds a story from storage.
func RestoreStory(id, authorID, photoID uuid.UUID, text StoryText, createdAt time.Time) *Story {
	return &Story{id: id, authorID: authorID, photoID: photoID, text: text, createdAt: createdAt.UTC()}
}

func (s *Story) ID() uuid.UUID {
	return s.id
}

func (s *Story) AuthorID() uuid.UUID {
	return s.authorID
}

func (s *Story) PhotoID() uuid.UUID {
	return s.photoID
}

func (s *Story) Text() StoryText {
	return s.text
}

func (s *Story) CreatedAt() time.Time {
	return s.createdAt
}

// ExpiresAt is when the story is gone.
func (s *Story) ExpiresAt() time.Time {
	return s.createdAt.Add(StoryLifetime)
}

// Live tells whether the story is shown at now.
func (s *Story) Live(now time.Time) bool {
	return now.Before(s.ExpiresAt())
}

// LiveSince is when the stories still shown at now were told: after it.
func LiveSince(now time.Time) time.Time {
	return now.Add(-StoryLifetime)
}

// Mine tells whether userID told the story.
func (s *Story) Mine(userID uuid.UUID) bool {
	return s.authorID == userID
}

// CheckDelete: only its author deletes a story.
func (s *Story) CheckDelete(requesterID uuid.UUID) error {
	if !s.Mine(requesterID) {
		return ErrCannotDeleteStory
	}

	return nil
}

// CheckViewers: only its author sees who viewed a story.
func (s *Story) CheckViewers(requesterID uuid.UUID) error {
	if !s.Mine(requesterID) {
		return ErrNotStoryAuthor
	}

	return nil
}

// CountsView tells whether viewerID viewing the story is counted: its
// author viewing their own is not.
func (s *Story) CountsView(viewerID uuid.UUID) bool {
	return viewerID != uuid.Nil && !s.Mine(viewerID)
}
