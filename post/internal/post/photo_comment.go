package post

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCannotDeletePhotoComment = errors.New("only the author of the comment, who uploaded the photo or the owner of the album can delete it")
	ErrReplyOtherPhoto          = errors.New("the comment to reply to is under another photo")
)

// PhotoComment is a comment on a photo of an album ("Комментарии" under
// a photo of openvk): text only — openvk takes attachments too. As a
// comment of a post it may answer another one of the same photo
// (replyTo, uuid.Nil for none, and once that one is deleted) and has
// hearts.
type PhotoComment struct {
	id        uuid.UUID
	photoID   uuid.UUID
	authorID  uuid.UUID
	text      CommentText
	replyTo   uuid.UUID
	likes     int
	createdAt time.Time
}

// NewPhotoComment comments photoID; its UUIDv7 ID grows with time, so the
// comments of a photo are ordered and paged by ID, oldest first.
func NewPhotoComment(photoID, authorID uuid.UUID, text CommentText, now time.Time) (*PhotoComment, error) {
	if text == "" {
		return nil, ErrCommentEmpty
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &PhotoComment{
		id: id, photoID: photoID, authorID: authorID, text: text,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// NewPhotoReply comments photoID in answer to parent, as openvk's
// "Ответить": parent must be under the same photo.
func NewPhotoReply(photoID uuid.UUID, parent *PhotoComment, authorID uuid.UUID, text CommentText, now time.Time) (*PhotoComment, error) {
	if parent.photoID != photoID {
		return nil, ErrReplyOtherPhoto
	}

	c, err := NewPhotoComment(photoID, authorID, text, now)
	if err != nil {
		return nil, err
	}

	c.replyTo = parent.id

	return c, nil
}

// StoredPhotoComment is a comment as storage keeps it.
type StoredPhotoComment struct {
	ID        uuid.UUID
	PhotoID   uuid.UUID
	AuthorID  uuid.UUID
	Text      CommentText
	ReplyTo   uuid.UUID
	Likes     int
	CreatedAt time.Time
}

// RestorePhotoComment rebuilds a comment from storage without validation.
func RestorePhotoComment(s StoredPhotoComment) *PhotoComment {
	return &PhotoComment{
		id: s.ID, photoID: s.PhotoID, authorID: s.AuthorID, text: s.Text,
		replyTo: s.ReplyTo, likes: s.Likes, createdAt: s.CreatedAt,
	}
}

// WithLikes returns the comment with n hearts: comments handed out never
// change.
func (c *PhotoComment) WithLikes(n int) *PhotoComment {
	counted := *c
	counted.likes = n

	return &counted
}

// WithoutReply returns the comment answering nothing, as it is once the
// comment it answered is deleted.
func (c *PhotoComment) WithoutReply() *PhotoComment {
	alone := *c
	alone.replyTo = uuid.Nil

	return &alone
}

// CanBeDeletedBy: its author, who uploaded the photo, or whoever may change
// the album (changesAlbum: its owner or the admins of its community).
func (c *PhotoComment) CanBeDeletedBy(userID uuid.UUID, photo *AlbumPhoto, changesAlbum bool) bool {
	return changesAlbum || userID == c.authorID || userID == photo.AuthorID()
}

func (c *PhotoComment) ID() uuid.UUID {
	return c.id
}

func (c *PhotoComment) PhotoID() uuid.UUID {
	return c.photoID
}

func (c *PhotoComment) AuthorID() uuid.UUID {
	return c.authorID
}

func (c *PhotoComment) Text() CommentText {
	return c.text
}

// ReplyTo is the comment this one answers; uuid.Nil for none.
func (c *PhotoComment) ReplyTo() uuid.UUID {
	return c.replyTo
}

func (c *PhotoComment) Likes() int {
	return c.likes
}

func (c *PhotoComment) CreatedAt() time.Time {
	return c.createdAt
}
