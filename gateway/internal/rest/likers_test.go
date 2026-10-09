package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestLikers(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	type page struct {
		Users []struct {
			Nickname string `json:"nickname"`
		} `json:"users"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}

	// Anyone sees them, newest first; a deleted page is left out, counted.
	w := h.do("GET", "/api/v1/posts/p1/likes?cursor=l1&limit=6", "")
	got := decode[page](t, w)
	if w.Code != http.StatusOK || len(got.Users) != 2 || got.Users[0].Nickname != "x1" || got.Users[1].Nickname != "bob" ||
		got.Total != 4 || got.NextCursor != "l2" || h.posts.likersOf != "p1" || h.posts.cursor != "l1" || h.posts.limit != 6 {
		t.Fatalf("%d %+v (asked %q %q %d)", w.Code, got, h.posts.likersOf, h.posts.cursor, h.posts.limit)
	}

	w = h.do("GET", "/api/v1/comments/c1/likes", "")
	if got = decode[page](t, w); w.Code != http.StatusOK || len(got.Users) != 1 || got.Total != 1 || h.posts.likersOf != "comment:c1" {
		t.Fatalf("comment: %d %+v", w.Code, got)
	}
}
