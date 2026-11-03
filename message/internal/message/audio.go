package message

import (
	"errors"

	"github.com/google/uuid"
)

// maxAudios is how many songs a message attaches.
const maxAudios = 10

var (
	ErrTooManyAudios  = errors.New("a message has at most 10 audios")
	ErrDuplicateAudio = errors.New("the same audio is attached twice")
	ErrInvalidAudio   = errors.New("invalid audio id")
)

// Audios are the IDs of songs of "Аудиозаписи" a message attaches, in the
// order they are shown. The post service keeps them; that they exist the
// gateway checks when the message is sent, and a deleted one is gone from
// the message too.
type Audios []uuid.UUID

func NewAudios(ids []uuid.UUID) (Audios, error) {
	if len(ids) > maxAudios {
		return nil, ErrTooManyAudios
	}

	if len(ids) == 0 {
		return nil, nil
	}

	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrInvalidAudio
		}
		if seen[id] {
			return nil, ErrDuplicateAudio
		}
		seen[id] = true
	}

	return Audios(ids), nil
}

// WithAudios is c attaching songs, restored from storage.
func (c Content) WithAudios(audios Audios) Content {
	c.audios = audios
	return c
}

// Audios are the songs the message attaches, nil for none.
func (c Content) Audios() Audios {
	return c.audios
}
