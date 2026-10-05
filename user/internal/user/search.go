package user

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	maxSearchQueryLen   = 64
	maxSearchQueryWords = 5
)

var (
	ErrSearchQueryEmpty   = errors.New("search query is empty")
	ErrSearchQueryTooLong = errors.New("search query is too long")
)

// SearchQuery is what people type to find someone: lower-cased words that
// must all occur in the name, surname or nickname, in any order.
type SearchQuery struct {
	words []string
}

func NewSearchQuery(s string) (SearchQuery, error) {
	if utf8.RuneCountInString(s) > maxSearchQueryLen {
		return SearchQuery{}, ErrSearchQueryTooLong
	}

	words := strings.Fields(strings.ToLower(s))
	if len(words) == 0 {
		return SearchQuery{}, ErrSearchQueryEmpty
	}

	if len(words) > maxSearchQueryWords {
		words = words[:maxSearchQueryWords]
	}

	return SearchQuery{words: words}, nil
}

func (q SearchQuery) Words() []string {
	return q.words
}

// Haystack is the text a user is searched by, in the same lower-cased form
// as the PostgreSQL index expression.
func Haystack(p Profile) string {
	return strings.ToLower(p.Nickname.String() + " " + p.Name.String() + " " + p.Surname.String())
}

// Matches reports whether every word of q occurs in the user's haystack.
func (q SearchQuery) Matches(p Profile) bool {
	haystack := Haystack(p)

	for _, w := range q.words {
		if !strings.Contains(haystack, w) {
			return false
		}
	}

	return true
}
