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

// Content is what a post shows: text, photos or both, never nothing.
type Content struct {
	text   Text
	photos Photos
}

func NewContent(text Text, photos Photos) (Content, error) {
	if text == "" && len(photos) == 0 {
		return Content{}, ErrTextEmpty
	}

	return Content{text: text, photos: photos}, nil
}

// RestoreContent rebuilds content from storage without validation.
func RestoreContent(text Text, photos Photos) Content {
	return Content{text: text, photos: photos}
}

func (c Content) Text() Text {
	return c.text
}

func (c Content) Photos() Photos {
	return c.photos
}
