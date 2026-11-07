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

// Content is what a post shows: text, photos, documents, a poll, a shared
// post (a repost) or some of them, never nothing.
type Content struct {
	text     Text
	photos   Photos
	repostOf uuid.UUID
	// polled: the post asks a poll (see Poll), which is content enough.
	polled bool
	// documents are files of "Мои Документы" the post attaches.
	documents Documents
	// audios are songs of "Аудиозаписи" the post attaches.
	audios Audios
	// link is the card of the page the text links to; zero for none.
	link Link
}

func NewContent(text Text, photos Photos) (Content, error) {
	if text == "" && len(photos) == 0 {
		return Content{}, ErrTextEmpty
	}

	return Content{text: text, photos: photos}, nil
}

// NewDocumented is the content of a post with documents: with them the
// text and the photos may be none.
func NewDocumented(text Text, photos Photos, documents Documents) (Content, error) {
	return NewAttached(text, photos, documents, nil)
}

// NewAttached is the content of a post with documents or songs: with any
// of them the text and the photos may be none.
func NewAttached(text Text, photos Photos, documents Documents, audios Audios) (Content, error) {
	if len(documents) == 0 && len(audios) == 0 {
		return NewContent(text, photos)
	}

	return Content{text: text, photos: photos, documents: documents, audios: audios}, nil
}

// WithDocuments is c attaching documents, restored from storage or of a
// post that asks a poll.
func (c Content) WithDocuments(documents Documents) Content {
	c.documents = documents
	return c
}

// WithAudios is c attaching songs, restored from storage or of a post
// that asks a poll.
func (c Content) WithAudios(audios Audios) Content {
	c.audios = audios
	return c
}

// Audios are the songs the post attaches.
func (c Content) Audios() Audios {
	return c.audios
}

// WithLink is c showing the card of the page its text links to; the
// text says the link, so the card is never all of it.
func (c Content) WithLink(link Link) Content {
	c.link = link
	return c
}

// Link is the card of the page the post links to, zero for none.
func (c Content) Link() Link {
	return c.link
}

// OwnPhotos are the photos of the post's author that go with the post:
// those it shows and the picture of its card.
func (c Content) OwnPhotos() Photos {
	if c.link.photoID == uuid.Nil {
		return c.photos
	}

	return append(append(Photos{}, c.photos...), c.link.photoID)
}

// Documents are the documents the post attaches.
func (c Content) Documents() Documents {
	return c.documents
}

// NewPolled is the content of a post that asks a poll: with it the text
// and the photos may be none.
func NewPolled(text Text, photos Photos) Content {
	return Content{text: text, photos: photos, polled: true}
}

// WithPoll is c of a post that asks a poll, restored from storage.
func (c Content) WithPoll() Content {
	c.polled = true
	return c
}

// Polled tells whether the post asks a poll.
func (c Content) Polled() bool {
	return c.polled
}

// NewRepost is the content of a repost: the shared post repostOf, with
// what the reposter says about it and the photos they add, if any.
func NewRepost(repostOf uuid.UUID, text Text, photos Photos) (Content, error) {
	if repostOf == uuid.Nil {
		return Content{}, ErrInvalidRepost
	}

	return Content{text: text, photos: photos, repostOf: repostOf}, nil
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
