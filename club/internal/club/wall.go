package club

import "errors"

var ErrInvalidWall = errors.New("invalid wall mode")

// Wall is who may write on a community's wall, as in openvk: everyone
// ("Открытая"), only its admins, who publish what others suggest
// ("Предложка"), or only its admins ("Закрытая"). Comments stay open
// whatever the wall.
type Wall string

const (
	WallOpen    Wall = "open"
	WallSuggest Wall = "suggest"
	WallClosed  Wall = "closed"
)

func NewWall(s string) (Wall, error) {
	switch w := Wall(s); w {
	case WallOpen, WallSuggest, WallClosed:
		return w, nil
	default:
		return "", ErrInvalidWall
	}
}

func (w Wall) String() string {
	return string(w)
}
