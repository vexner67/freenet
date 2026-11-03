package fedi

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxCommentText bounds a comment of a remote post, as a comment of a
// post of the site.
const MaxCommentText = 60_000

// ErrCommentText: a comment says nothing, or more than MaxCommentText.
var ErrCommentText = errors.New("comment text is empty or too long")

// RemoteComment is what a person here wrote under a remote post. It stays
// here: the server of the post does not learn of it.
type RemoteComment struct {
	id        uuid.UUID
	postID    uuid.UUID
	authorID  uuid.UUID
	text      string
	createdAt time.Time
}

// NewRemoteComment is the comment text of authorID under the remote post
// postID, written at now; ErrCommentText. Its UUIDv7 ID grows with time
// and pages the comments.
func NewRemoteComment(postID, authorID uuid.UUID, text string, now time.Time) (*RemoteComment, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > MaxCommentText {
		return nil, ErrCommentText
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &RemoteComment{id: id, postID: postID, authorID: authorID, text: text, createdAt: now.UTC().Truncate(time.Microsecond)}, nil
}

// RestoreRemoteComment is a comment as it was kept.
func RestoreRemoteComment(id, postID, authorID uuid.UUID, text string, createdAt time.Time) *RemoteComment {
	return &RemoteComment{id: id, postID: postID, authorID: authorID, text: text, createdAt: createdAt.UTC()}
}

func (c *RemoteComment) ID() uuid.UUID        { return c.id }
func (c *RemoteComment) PostID() uuid.UUID    { return c.postID }
func (c *RemoteComment) AuthorID() uuid.UUID  { return c.authorID }
func (c *RemoteComment) Text() string         { return c.text }
func (c *RemoteComment) CreatedAt() time.Time { return c.createdAt }
