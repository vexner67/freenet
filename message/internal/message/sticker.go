package message

import (
	"errors"
	"regexp"
)

var (
	ErrInvalidSticker = errors.New("invalid sticker")
	// ErrStickerAlone: a sticker is a message by itself, as on VK: no
	// words, photos or anything else go with it, and it is not edited.
	ErrStickerAlone = errors.New("a sticker goes alone")
)

// stickerPattern is the shape of a sticker's name in the catalogue.
var stickerPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// Sticker names a sticker of openvk's "Стикеры", "" for none. The
// catalogue and who has which pack are the user service's: the message
// keeps the name, and the caller has checked the sender may send it.
type Sticker string

func NewSticker(s string) (Sticker, error) {
	if !stickerPattern.MatchString(s) {
		return "", ErrInvalidSticker
	}

	return Sticker(s), nil
}

func (s Sticker) String() string {
	return string(s)
}

// NewStickerContent is the content of a message that is a sticker and
// nothing else.
func NewStickerContent(s Sticker) (Content, error) {
	if s == "" {
		return Content{}, ErrInvalidSticker
	}

	return Content{sticker: s}, nil
}

// WithSticker is c showing s, restored from storage.
func (c Content) WithSticker(s Sticker) Content {
	c.sticker = s
	return c
}

// Sticker is the sticker the message is, "" for none.
func (c Content) Sticker() Sticker {
	return c.sticker
}
