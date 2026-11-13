package post_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewAppSection(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want post.AppSection
		err  error
	}{
		{in: "", want: post.AppsAll},
		{in: "all", want: post.AppsAll},
		{in: "installed", want: post.AppsInstalled},
		{in: "own", want: post.AppsOwn},
		{in: "dev", err: post.ErrInvalidAppSection},
	} {
		got, err := post.NewAppSection(tt.in)
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("NewAppSection(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
		}
	}
}
