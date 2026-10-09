package post

import (
	"errors"
	"strings"
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
