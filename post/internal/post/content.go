package post

import (
	"errors"

	"github.com/google/uuid"
)

const maxPhotos = 10

var (
	ErrTooManyPhotos  = errors.New("a post has at most 10 photos")
	ErrDuplicatePhoto = errors.New("the same photo is attached twice")
	ErrInvalidPhoto   = errors.New("invalid photo id")
	ErrInvalidRepost  = errors.New("invalid post id to share")
)

// Photos are the IDs of a post's photos in the media service, in the order
// they are shown. Whose photos they are is checked when the post is
// written: the gateway asks the media service.
type Photos []uuid.UUID

func NewPhotos(ids []uuid.UUID) (Photos, error) {
	if len(ids) > maxPhotos {
		return nil, ErrTooManyPhotos
	}

	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrInvalidPhoto
		}
		if seen[id] {
			return nil, ErrDuplicatePhoto
		}
		seen[id] = true
	}

	return Photos(ids), nil
}

// Content is what a post shows: text, photos, a shared post (a repost)
// or some of them, or a shared photo of an album with text, never nothing.
type Content struct {
	text     Text
	photos   Photos
	repostOf uuid.UUID
	// sharedPhoto is a photo of an album shared ("Поделиться" of a photo),
	// uuid.Nil for none; only with text, never with photos or a repost.
	sharedPhoto uuid.UUID
}

func NewContent(text Text, photos Photos) (Content, error) {
	if text == "" && len(photos) == 0 {
		return Content{}, ErrTextEmpty
	}

	return Content{text: text, photos: photos}, nil
}

// NewRepost is the content of a repost: the shared post repostOf, with
// what the reposter says about it, if anything.
func NewRepost(repostOf uuid.UUID, text Text) (Content, error) {
	if repostOf == uuid.Nil {
		return Content{}, ErrInvalidRepost
	}

	return Content{text: text, repostOf: repostOf}, nil
}

// NewPhotoShare is the content of a post sharing a photo of an album, with
// what the poster says about it, if anything.
func NewPhotoShare(photoID uuid.UUID, text Text) (Content, error) {
	if photoID == uuid.Nil {
		return Content{}, ErrInvalidPhoto
	}

	return Content{text: text, sharedPhoto: photoID}, nil
}

// WithSharedPhoto is c sharing photoID, restored from storage.
func (c Content) WithSharedPhoto(photoID uuid.UUID) Content {
	c.sharedPhoto = photoID
	return c
}

// SharedPhoto is the photo of an album the post shares, uuid.Nil for none.
func (c Content) SharedPhoto() uuid.UUID {
	return c.sharedPhoto
}

// RestoreContent rebuilds content from storage without validation;
// repostOf is uuid.Nil but for reposts.
func RestoreContent(text Text, photos Photos, repostOf uuid.UUID) Content {
	return Content{text: text, photos: photos, repostOf: repostOf}
}

func (c Content) Text() Text {
	return c.text
}

func (c Content) Photos() Photos {
	return c.photos
}

// RepostOf is the shared post; uuid.Nil when the post is not a repost.
func (c Content) RepostOf() uuid.UUID {
	return c.repostOf
}

// passesOn tells a repost that adds nothing to the post it shares.
func (c Content) passesOn() bool {
	return c.repostOf != uuid.Nil && c.text == "" && len(c.photos) == 0
}
