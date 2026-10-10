package post

import (
	"errors"
	"slices"

	"github.com/google/uuid"
)

// MaxMentions is how many people one text tells that it names them.
const MaxMentions = 10

var ErrTooManyMentions = errors.New("a text names at most 10 people")

// Mentions are the people a post, a comment or a message of a topic
// names ("@nickname"), who hear of it as openvk tells "Вас упомянул
// пользователь". Who they are the caller has found out: post reads no
// nicknames.
type Mentions []uuid.UUID

// NewMentions keeps each person once, in the order named.
func NewMentions(ids []uuid.UUID) (Mentions, error) {
	var m Mentions
	for _, id := range ids {
		if id != uuid.Nil && !slices.Contains(m, id) {
			m = append(m, id)
		}
	}

	if len(m) > MaxMentions {
		return nil, ErrTooManyMentions
	}

	return m, nil
}

// Heard are the named who are told of it: not its author, nor those who
// hear of the text anyway (already), as the owner of the wall it is on or
// whoever it answers.
func (m Mentions) Heard(authorID uuid.UUID, already ...uuid.UUID) []uuid.UUID {
	var heard []uuid.UUID
	for _, id := range m {
		if id != authorID && !slices.Contains(already, id) {
			heard = append(heard, id)
		}
	}

	return heard
}
