package user

import "errors"

type PoliticalViews string

const (
	PoliticalViewsNone              PoliticalViews = ""
	PoliticalViewsCommunist         PoliticalViews = "communist"
	PoliticalViewsSocialist         PoliticalViews = "socialist"
	PoliticalViewsModerate          PoliticalViews = "moderate"
	PoliticalViewsLiberal           PoliticalViews = "liberal"
	PoliticalViewsConservative      PoliticalViews = "conservative"
	PoliticalViewsMonarchist        PoliticalViews = "monarchist"
	PoliticalViewsUltraconservative PoliticalViews = "ultraconservative"
	PoliticalViewsIndifferent       PoliticalViews = "indifferent"
	PoliticalViewsLibertarian       PoliticalViews = "libertarian"
)

var ErrInvalidPoliticalViews = errors.New("invalid political views")

func NewPoliticalViews(s string) (PoliticalViews, error) {
	switch PoliticalViews(s) {
	case PoliticalViewsNone,
		PoliticalViewsCommunist,
		PoliticalViewsSocialist,
		PoliticalViewsModerate,
		PoliticalViewsLiberal,
		PoliticalViewsConservative,
		PoliticalViewsMonarchist,
		PoliticalViewsUltraconservative,
		PoliticalViewsIndifferent,
		PoliticalViewsLibertarian:
		return PoliticalViews(s), nil
	default:
		return "", ErrInvalidPoliticalViews
	}
}

func (p PoliticalViews) String() string {
	return string(p)
}
