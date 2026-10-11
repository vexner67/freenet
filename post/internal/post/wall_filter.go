package post

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// WallFilter picks the posts of a wall, as the tabs of openvk's wall:
// "Все записи", "Записи Анны" (of a community, "Записи сообщества"),
// "Чужие записи" and "Архив записей", which the others leave out.
type WallFilter string

const (
	WallAll     WallFilter = ""
	WallOwner   WallFilter = "owner"
	WallOthers  WallFilter = "others"
	WallArchive WallFilter = "archive"
)

// maxWallQueryLen bounds what a wall is searched for.
const maxWallQueryLen = 100

var (
	ErrInvalidWallFilter = errors.New("invalid wall filter")
	ErrWallQueryTooLong  = errors.New("search query is too long")
	ErrInvalidWallYear   = errors.New("invalid year")
)

func NewWallFilter(s string) (WallFilter, error) {
	switch WallFilter(s) {
	case WallAll, WallOwner, WallOthers, WallArchive:
		return WallFilter(s), nil
	default:
		return "", ErrInvalidWallFilter
	}
}

// Matches tells whether p is among the posts f picks: the archive shows
// the posts put away, every other tab those that are not.
func (f WallFilter) Matches(p *Post) bool {
	if p.archived != (f == WallArchive) {
		return false
	}

	switch f {
	case WallOwner:
		return p.ByWallOwner()
	case WallOthers:
		return !p.ByWallOwner()
	default:
		return true
	}
}

// ByWallOwner: the post is the wall's own, written by its owner on their
// wall or speaking for the community whose wall it is.
func (p *Post) ByWallOwner() bool {
	return p.authorID == p.wallID || p.fromWall
}

// WallQuery is what a wall is searched for ("Поиск" of openvk's wall):
// trimmed text the posts say, in any case; empty searches nothing.
type WallQuery string

func NewWallQuery(s string) (WallQuery, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxWallQueryLen {
		return "", ErrWallQueryTooLong
	}

	return WallQuery(s), nil
}

func (q WallQuery) String() string {
	return string(q)
}

// Matches tells whether p says q; every post matches the empty query.
func (q WallQuery) Matches(p *Post) bool {
	return q == "" || strings.Contains(strings.ToLower(p.content.text.String()), strings.ToLower(string(q)))
}

// minWallYear and maxWallYear bound the years a wall is asked about: no
// post is older than the site, and a timestamp holds no year above 9999.
const (
	minWallYear = 1970
	maxWallYear = 9999
)

// WallYear picks the posts of a wall written in one year, by the clock of
// UTC, as the years of openvk's archive do; 0 picks every year.
type WallYear int

func NewWallYear(year int) (WallYear, error) {
	if year != 0 && (year < minWallYear || year > maxWallYear) {
		return 0, ErrInvalidWallYear
	}

	return WallYear(year), nil
}

// YearOf is the year at was written in.
func YearOf(at time.Time) WallYear {
	return WallYear(at.UTC().Year())
}

// Span is the time the year takes: from its start to the start of the
// next; ok is false for every year.
func (y WallYear) Span() (from, to time.Time, ok bool) {
	if y == 0 {
		return time.Time{}, time.Time{}, false
	}

	from = time.Date(int(y), time.January, 1, 0, 0, 0, 0, time.UTC)

	return from, from.AddDate(1, 0, 0), true
}

// Matches tells whether p was written in the year.
func (y WallYear) Matches(p *Post) bool {
	return y == 0 || YearOf(p.createdAt) == y
}
