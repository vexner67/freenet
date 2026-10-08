package post

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const maxCommentLen = 1000

var (
	// ErrCommentEmpty: neither text nor photos. The message keeps the
	// words it had before comments took photos: clients know it.
	ErrCommentEmpty        = errors.New("comment text is empty")
	ErrCommentTooLong      = errors.New("comment text is too long")
	ErrCommentNotFound     = errors.New("comment not found")
	ErrCannotDeleteComment = errors.New("only the author of the comment or of the post or the owner of the wall can delete it")
	ErrReplyOtherPost      = errors.New("the comment to reply to is under another post")
)

// CommentText is what a comment says: trimmed, with "\n" line breaks. It
// may be empty when the comment has photos (see NewCommentContent).
type CommentText string

func NewCommentText(s string) (CommentText, error) {
	s, err := cleanText(s, maxCommentLen, nil, ErrCommentTooLong)
	if err != nil {
		return "", err
	}

	return CommentText(s), nil
}

func (t CommentText) String() string {
	return string(t)
}

// CommentContent is what a comment shows: text, photos or both, never
// nothing, as in openvk.
type CommentContent struct {
	text   CommentText
	photos Photos
}

func NewCommentContent(text CommentText, photos Photos) (CommentContent, error) {
	if text == "" && len(photos) == 0 {
		return CommentContent{}, ErrCommentEmpty
	}

	return CommentContent{text: text, photos: photos}, nil
}

// RestoreCommentContent rebuilds content from storage without validation.
func RestoreCommentContent(text CommentText, photos Photos) CommentContent {
	return CommentContent{text: text, photos: photos}
}

func (c CommentContent) Text() CommentText {
	return c.text
}

func (c CommentContent) Photos() Photos {
	return c.photos
}

type Comment struct {
	id       uuid.UUID
	postID   uuid.UUID
	authorID uuid.UUID
	// replyTo is the comment this one answers, uuid.Nil for none; it may
	// be gone since.
	replyTo   uuid.UUID
	content   CommentContent
	createdAt time.Time
	likes     int
}

// NewComment creates a comment on a post. Its UUIDv7 ID grows with time,
// so the comments of a post are ordered and paged by ID.
func NewComment(postID, authorID uuid.UUID, content CommentContent, now time.Time) (*Comment, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Comment{
		id:       id,
		postID:   postID,
		authorID: authorID,
		content:  content,
		// PostgreSQL keeps microseconds; trim now so a comment reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// NewReply creates a comment on postID that answers parent, as openvk's
// "Ответить": parent must be under the same post.
func NewReply(postID uuid.UUID, parent *Comment, authorID uuid.UUID, content CommentContent, now time.Time) (*Comment, error) {
	if parent.postID != postID {
		return nil, ErrReplyOtherPost
	}

	c, err := NewComment(postID, authorID, content, now)
	if err != nil {
		return nil, err
	}

	c.replyTo = parent.id

	return c, nil
}

// StoredComment is a comment as storage keeps it.
type StoredComment struct {
	ID        uuid.UUID
	PostID    uuid.UUID
	AuthorID  uuid.UUID
	ReplyTo   uuid.UUID
	Content   CommentContent
	CreatedAt time.Time
	Likes     int
}

// RestoreComment rebuilds a comment from storage without validation or new IDs.
func RestoreComment(s StoredComment) *Comment {
	return &Comment{
		id:        s.ID,
		postID:    s.PostID,
		authorID:  s.AuthorID,
		replyTo:   s.ReplyTo,
		content:   s.Content,
		createdAt: s.CreatedAt,
		likes:     s.Likes,
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

// ReplyTo is the comment this one answers; uuid.Nil for none.
func (c *Comment) ReplyTo() uuid.UUID {
	return c.replyTo
}

// Likes is how many people like the comment.
func (c *Comment) Likes() int {
	return c.likes
}

func (c *Comment) Text() CommentText {
	return c.content.text
}

func (c *Comment) Photos() Photos {
	return c.content.photos
}

func (c *Comment) CreatedAt() time.Time {
	return c.createdAt
}
