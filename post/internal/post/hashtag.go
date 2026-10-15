package post

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits of hashtags: a tag longer than maxHashtagLen is no tag, and a
// text is found by its first MaxHashtags tags.
const (
	maxHashtagLen = 64
	MaxHashtags   = 30
)

// maxSearchQueryLen bounds what posts are searched for.
const maxSearchQueryLen = 100

var ErrSearchQueryTooLong = errors.New("search query is too long")

// Hashtag is a word a text marks with "#" to be found by ("#поход"),
// which openvk links to the search of posts: "#" at the start of the text
// or after a space, then letters, digits and "_". It is kept in lower
// case, without "#": a tag is found in any case.
type Hashtag string

func (h Hashtag) String() string {
	return string(h)
}

// Hashtags are the tags t says, each once, in the order said, at most
// MaxHashtags of them.
func (t Text) Hashtags() []Hashtag {
	s := string(t)

	var tags []Hashtag
	for i := 0; i < len(s) && len(tags) < MaxHashtags; {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r != '#' || (i > 0 && !spaceBefore(s[:i])) {
			i += size
			continue
		}

		word := tagWord(s[i+size:])
		i += size + len(word)

		tag := Hashtag(strings.ToLower(word))
		if tag != "" && utf8.RuneCountInString(word) <= maxHashtagLen && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}

	return tags
}

// spaceBefore tells whether s ends with a space.
func spaceBefore(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(r)
}

// tagWord is the start of s a tag is made of.
func tagWord(s string) string {
	end := strings.IndexFunc(s, func(r rune) bool { return !isTagRune(r) })
	if end < 0 {
		return s
	}

	return s[:end]
}

func isTagRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || r == '_'
}

// SearchQuery is what the posts of every wall are searched for (openvk's
// "Поиск записей"): one tag ("#поход") finds the posts that say it, any
// other trimmed text those that say it as a part of theirs, in any case.
type SearchQuery string

func NewSearchQuery(s string) (SearchQuery, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxSearchQueryLen {
		return "", ErrSearchQueryTooLong
	}

	return SearchQuery(s), nil
}

func (q SearchQuery) String() string {
	return string(q)
}

// Hashtag is the tag q is, if it is one tag and nothing else.
func (q SearchQuery) Hashtag() (Hashtag, bool) {
	tags := Text(q).Hashtags()
	if len(tags) != 1 || strings.ToLower(string(q)) != "#"+tags[0].String() {
		return "", false
	}

	return tags[0], true
}

// Matches tells whether p is found by q, which is not empty.
func (q SearchQuery) Matches(p *Post) bool {
	if tag, ok := q.Hashtag(); ok {
		return slices.Contains(p.content.text.Hashtags(), tag)
	}

	return strings.Contains(strings.ToLower(p.content.text.String()), strings.ToLower(string(q)))
}
