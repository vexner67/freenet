package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewPoliticalViews(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", "liberal", "libertarian", "moderate_islamism", "radical_islamism"} {
		if got, err := user.NewPoliticalViews(s); err != nil || got.String() != s {
			t.Fatalf("NewPoliticalViews(%q) = %q, %v", s, got, err)
		}
	}

	// "moderate" was taken off the list.
	for _, s := range []string{"moderate", "Liberal", "anarchist"} {
		if _, err := user.NewPoliticalViews(s); !errors.Is(err, user.ErrInvalidPoliticalViews) {
			t.Fatalf("NewPoliticalViews(%q): %v, want ErrInvalidPoliticalViews", s, err)
		}
	}
}
