package post

import "errors"

// WallFilter picks the posts of a wall, as the tabs of openvk's wall:
// "Все записи", "Записи Анны" (of a community, "Записи сообщества") and
// "Чужие записи".
type WallFilter string

const (
	WallAll    WallFilter = ""
	WallOwner  WallFilter = "owner"
	WallOthers WallFilter = "others"
)

var ErrInvalidWallFilter = errors.New("invalid wall filter")

func NewWallFilter(s string) (WallFilter, error) {
	switch WallFilter(s) {
	case WallAll, WallOwner, WallOthers:
		return WallFilter(s), nil
	default:
		return "", ErrInvalidWallFilter
	}
}

// Matches tells whether p is among the posts f picks.
func (f WallFilter) Matches(p *Post) bool {
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
