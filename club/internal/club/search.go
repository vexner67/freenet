package club

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

// SearchQuery is what people type to find a community: lower-cased words
// that must all occur in its name or address, in any order.
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

// Matches reports whether every word of q occurs in the name or address of
// c, as the PostgreSQL search does.
func (q SearchQuery) Matches(c *Club) bool {
	haystack := strings.ToLower(c.name.String() + " " + c.address.String())

	for _, w := range q.words {
		if !strings.Contains(haystack, w) {
			return false
		}
	}

	return true
}
