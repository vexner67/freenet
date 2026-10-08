package club

import "errors"

var ErrInvalidWall = errors.New("invalid wall mode")

// Wall is who may write on a community's wall, as in openvk: everyone
// ("Открытая") or only its admins ("Закрытая"). Comments stay open either
// way.
type Wall string

const (
	WallOpen   Wall = "open"
	WallClosed Wall = "closed"
)

func NewWall(s string) (Wall, error) {
	switch w := Wall(s); w {
	case WallOpen, WallClosed:
		return w, nil
	default:
		return "", ErrInvalidWall
	}
}

func (w Wall) String() string {
	return string(w)
}
