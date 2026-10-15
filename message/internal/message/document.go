package message

import (
	"errors"

	"github.com/google/uuid"
)

// maxDocuments is how many documents a message attaches.
const maxDocuments = 10

var (
	ErrTooManyDocuments  = errors.New("a message has at most 10 documents")
	ErrDuplicateDocument = errors.New("the same document is attached twice")
	ErrInvalidDocument   = errors.New("invalid document id")
)

// Documents are the IDs of files of "Мои Документы" a message attaches,
// in the order they are shown. The post service keeps them; whose they
// are the gateway checks when the message is sent, and a deleted one is
// gone from the message too.
type Documents []uuid.UUID

func NewDocuments(ids []uuid.UUID) (Documents, error) {
	if len(ids) > maxDocuments {
		return nil, ErrTooManyDocuments
	}

	if len(ids) == 0 {
		return nil, nil
	}

	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrInvalidDocument
		}
		if seen[id] {
			return nil, ErrDuplicateDocument
		}
		seen[id] = true
	}

	return Documents(ids), nil
}

// NewDocumentContent is the content of a message of documents alone.
func NewDocumentContent(documents Documents) (Content, error) {
	if len(documents) == 0 {
		return Content{}, ErrTextEmpty
	}

	return Content{documents: documents}, nil
}

// Attaching is c with documents: anything but a sticker, which goes alone.
func (c Content) Attaching(documents Documents) (Content, error) {
	if len(documents) > 0 && c.sticker != "" {
		return Content{}, ErrStickerAlone
	}

	c.documents = documents

	return c, nil
}

// WithDocuments is c attaching documents, restored from storage.
func (c Content) WithDocuments(documents Documents) Content {
	c.documents = documents
	return c
}

// Documents are the documents the message attaches, nil for none.
func (c Content) Documents() Documents {
	return c.documents
}
