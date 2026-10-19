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
// that must all occur in its name or address, in any order. A query of
// the verified clubs finds only those, and may have no words.
type SearchQuery struct {
	words    []string
	verified bool
}

func NewSearchQuery(s string) (SearchQuery, error) {
	words, err := searchWords(s)
	if err != nil {
		return SearchQuery{}, err
	}

	if len(words) == 0 {
		return SearchQuery{}, ErrSearchQueryEmpty
	}

	return SearchQuery{words: words}, nil
}

// NewVerifiedQuery finds the verified clubs, of them those s names when
// it is not blank: the list the administrators of the site keep.
func NewVerifiedQuery(s string) (SearchQuery, error) {
	words, err := searchWords(s)
	if err != nil {
		return SearchQuery{}, err
	}

	return SearchQuery{words: words, verified: true}, nil
}

func searchWords(s string) ([]string, error) {
	if utf8.RuneCountInString(s) > maxSearchQueryLen {
		return nil, ErrSearchQueryTooLong
	}

	words := strings.Fields(strings.ToLower(s))
	if len(words) > maxSearchQueryWords {
		words = words[:maxSearchQueryWords]
	}

	return words, nil
}

func (q SearchQuery) Words() []string {
	return q.words
}

// Verified tells whether q finds only the verified clubs.
func (q SearchQuery) Verified() bool {
	return q.verified
}

// Matches reports whether every word of q occurs in the name or address of
// c, and c is verified if q asks for that, as the PostgreSQL search does.
func (q SearchQuery) Matches(c *Club) bool {
	if q.verified && !c.verified {
		return false
	}

	haystack := strings.ToLower(c.name.String() + " " + c.address.String())

	for _, w := range q.words {
		if !strings.Contains(haystack, w) {
			return false
		}
	}

	return true
}
