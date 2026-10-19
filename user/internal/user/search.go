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
// must all occur in the name, surname or nickname, in any order, and the
// filters openvk's page links to.
type SearchQuery struct {
	words   []string
	filters SearchFilters
}

// SearchFilters narrow a search as the links of openvk's page do: people
// of a hometown or a city (whatever the case), or who name a film, a show
// or a book among their favourites. Values are lower-cased; "" filters
// nothing. Verified keeps the pages with the checkmark only, for the
// administrators' list of people.
type SearchFilters struct {
	Hometown string
	City     string
	Film     string
	Show     string
	Book     string
	Verified bool
}

// LookIntoInfo tells whether the filters read the information of a page,
// which only the pages open to everyone tell; the checkmark is no
// information.
func (f SearchFilters) LookIntoInfo() bool {
	f.Verified = false

	return f != SearchFilters{}
}

// NewSearchQuery takes the words and the filters; without filters the
// words are required.
func NewSearchQuery(s string, f SearchFilters) (SearchQuery, error) {
	if utf8.RuneCountInString(s) > maxSearchQueryLen {
		return SearchQuery{}, ErrSearchQueryTooLong
	}

	for _, v := range []*string{&f.Hometown, &f.City, &f.Film, &f.Show, &f.Book} {
		*v = strings.ToLower(strings.Join(strings.Fields(*v), " "))
		if utf8.RuneCountInString(*v) > maxSearchQueryLen {
			return SearchQuery{}, ErrSearchQueryTooLong
		}
	}

	words := strings.Fields(strings.ToLower(s))
	if len(words) == 0 && f == (SearchFilters{}) {
		return SearchQuery{}, ErrSearchQueryEmpty
	}

	if len(words) > maxSearchQueryWords {
		words = words[:maxSearchQueryWords]
	}

	return SearchQuery{words: words, filters: f}, nil
}

func (q SearchQuery) Words() []string {
	return q.words
}

func (q SearchQuery) Filters() SearchFilters {
	return q.filters
}

// Haystack is the text a user is searched by, in the same lower-cased form
// as the PostgreSQL index expression.
func Haystack(p Profile) string {
	return strings.ToLower(p.Nickname.String() + " " + p.Name.String() + " " + p.Surname.String())
}

// Matches reports whether every word of q occurs in the user's haystack
// and the user passes its filters.
func (q SearchQuery) Matches(u *User) bool {
	haystack := Haystack(u.profile)

	for _, w := range q.words {
		if !strings.Contains(haystack, w) {
			return false
		}
	}

	// The filters look into the information, which only the pages open to
	// everyone tell.
	f := q.filters
	if f.LookIntoInfo() && (u.privacy.InfoRead != AudienceEveryone || u.privacy.Closed) {
		return false
	}

	if f.Verified && !u.verified {
		return false
	}

	same := func(want, got string) bool { return want == "" || want == strings.ToLower(got) }
	names := func(want string, got About) bool {
		return want == "" || strings.Contains(strings.ToLower(got.String()), want)
	}

	return same(f.Hometown, u.profile.Hometown.String()) && same(f.City, u.contacts.City.String()) &&
		names(f.Film, u.interests.Films) && names(f.Show, u.interests.Shows) && names(f.Book, u.interests.Books)
}
