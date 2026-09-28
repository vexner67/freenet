package user

import "errors"

type Worldview string

const (
	WorldviewNone            Worldview = ""
	WorldviewIslam           Worldview = "islam"
	WorldviewOrthodox        Worldview = "orthodox"
	WorldviewCatholicism     Worldview = "catholicism"
	WorldviewJudaism         Worldview = "judaism"
	WorldviewProtestantism   Worldview = "protestantism"
	WorldviewBuddhism        Worldview = "buddhism"
	WorldviewConfucianism    Worldview = "confucianism"
	WorldviewSecularHumanism Worldview = "secular_humanism"
)

var ErrInvalidWorldview = errors.New("invalid worldview")

func NewWorldview(s string) (Worldview, error) {
	if s == "" {
		return "", nil
	}

	switch Worldview(s) {
	case WorldviewNone,
		WorldviewIslam,
		WorldviewOrthodox,
		WorldviewCatholicism,
		WorldviewJudaism,
		WorldviewProtestantism,
		WorldviewBuddhism,
		WorldviewConfucianism,
		WorldviewSecularHumanism:
		return Worldview(s), nil
	default:
		return "", ErrInvalidWorldview
	}
}

func (w Worldview) String() string {
	return string(w)
}
