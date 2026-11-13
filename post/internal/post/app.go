package post

import "errors"

// AppSection is a list of "Приложения", as openvk's tabs: all the apps
// someone added, those the viewer added, and the viewer's own widgets.
type AppSection string

const (
	AppsAll       AppSection = "all"
	AppsInstalled AppSection = "installed"
	AppsOwn       AppSection = "own"
)

var ErrInvalidAppSection = errors.New("invalid app section")

// NewAppSection reads a section; "" is all of them.
func NewAppSection(s string) (AppSection, error) {
	switch AppSection(s) {
	case "", AppsAll:
		return AppsAll, nil
	case AppsInstalled, AppsOwn:
		return AppSection(s), nil
	default:
		return "", ErrInvalidAppSection
	}
}
