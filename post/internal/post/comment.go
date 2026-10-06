package post

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const maxCommentLen = 1000

var (
	ErrCommentEmpty        = errors.New("comment text is empty")
	ErrCommentTooLong      = errors.New("comment text is too long")
	ErrCommentNotFound     = errors.New("comment not found")
	ErrCannotDeleteComment = errors.New("only the author of the comment or of the post or the owner of the wall can delete it")
)

// CommentText is what a comment says: trimmed, with "\n" line breaks.
type CommentText string

func NewCommentText(s string) (CommentText, error) {
	s, err := cleanText(s, maxCommentLen, ErrCommentEmpty, ErrCommentTooLong)
	if err != nil {
		return "", err
	}

	return CommentText(s), nil
}

func (t CommentText) String() string {
	return string(t)
}

type Comment struct {
	id        uuid.UUID
	postID    uuid.UUID
	authorID  uuid.UUID
	text      CommentText
	createdAt time.Time
}

// NewComment creates a comment on a post. Its UUIDv7 ID grows with time,
// so the comments of a post are ordered and paged by ID.
func NewComment(postID, authorID uuid.UUID, text CommentText, now time.Time) (*Comment, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Comment{
		id:       id,
		postID:   postID,
		authorID: authorID,
		text:     text,
		// PostgreSQL keeps microseconds; trim now so a comment reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// RestoreComment rebuilds a comment from storage without validation or new IDs.
func RestoreComment(id, postID, authorID uuid.UUID, text CommentText, createdAt time.Time) *Comment {
	return &Comment{
		id:        id,
		postID:    postID,
		authorID:  authorID,
		text:      text,
		createdAt: createdAt,
	}
}

// CanBeDeletedBy: a comment is removed by whoever wrote it, by the author
// of the post, who looks after the discussion, or by the owner of the wall,
// who keeps the wall clean. p is the post of the comment.
func (c *Comment) CanBeDeletedBy(userID uuid.UUID, p *Post) bool {
	return userID == c.authorID || userID == p.AuthorID() || userID == p.WallID()
}

func (c *Comment) ID() uuid.UUID {
	return c.id
}

func (c *Comment) PostID() uuid.UUID {
	return c.postID
}

func (c *Comment) AuthorID() uuid.UUID {
	return c.authorID
}

func (c *Comment) Text() CommentText {
	return c.text
}

func (c *Comment) CreatedAt() time.Time {
	return c.createdAt
}
