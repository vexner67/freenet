package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeFaves likes posts and comments of u1 (ann), u2 (bob) and u3
// (a deleted page), and wrote a post.
type fakeFaves struct {
	asked, token, cursor string
	limit                int
}

var faveTime = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func (f *fakeFaves) ask(what, token, cursor string, limit int) {
	f.asked, f.token, f.cursor, f.limit = what, token, cursor, limit
}

func (f *fakeFaves) Posts(_ context.Context, token, cursor string, limit int) (model.PostPage, error) {
	f.ask("posts", token, cursor, limit)

	return model.PostPage{Posts: []model.Post{
		{ID: "p5", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u1"}, Text: "у Анны", Liked: true, Likes: 1, CreatedAt: faveTime},
		{ID: "p6", Author: model.Author{ID: "u3"}, Wall: model.Author{ID: "u3"}, Text: "удалённой", Liked: true, CreatedAt: faveTime},
	}, NextCursor: "1_p6", Total: 7}, nil
}

func (f *fakeFaves) OwnPosts(_ context.Context, token, cursor string, limit int) (model.PostPage, error) {
	f.ask("own", token, cursor, limit)

	return model.PostPage{Posts: []model.Post{
		{ID: "p7", Author: model.Author{ID: "u1"}, Wall: model.Author{ID: "u2"}, Text: "у Петра", Mine: true, CreatedAt: faveTime},
	}, Total: 1}, nil
}

func (f *fakeFaves) Comments(_ context.Context, token, cursor string, limit int) (model.CommentPage, error) {
	f.ask("comments", token, cursor, limit)

	return model.CommentPage{Comments: []model.Comment{
		{ID: "c5", PostID: "p1", Author: model.Author{ID: "u2"}, Text: "согласен", Liked: true, Likes: 1, CreatedAt: faveTime},
		{ID: "c6", PostID: "p1", Author: model.Author{ID: "u3"}, Text: "удалённой", Liked: true, CreatedAt: faveTime},
		{ID: "c7", PostID: "p404", Author: model.Author{ID: "u2"}, Text: "под пропавшей записью", Liked: true, CreatedAt: faveTime},
	}, NextCursor: "1_c7", Total: 3}, nil
}

type favePostsBody struct {
	Posts []struct {
		ID     string         `json:"id"`
		Author map[string]any `json:"author"`
		Wall   map[string]any `json:"wall"`
		Liked  bool           `json:"liked"`
		Mine   bool           `json:"mine"`
	} `json:"posts"`
	NextCursor string `json:"nextCursor"`
	Total      int    `json:"total"`
}

func TestListFavePosts(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/me/faves/posts?cursor=1_p9&limit=5", "", "Authorization", "Bearer tok")
	got := decode[favePostsBody](t, w)

	// The post of the deleted page is left out, but counted.
	if w.Code != http.StatusOK || got.Total != 7 || got.NextCursor != "1_p6" || len(got.Posts) != 1 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if p := got.Posts[0]; p.ID != "p5" || !p.Liked || p.Author["id"] != "u2" || p.Wall["id"] != "u1" {
		t.Fatalf("post = %+v", p)
	}
	if h.faves.asked != "posts" || h.faves.token != "tok" || h.faves.cursor != "1_p9" || h.faves.limit != 5 {
		t.Fatalf("request = %+v", h.faves)
	}

	// The faves are one's own: nobody reads them without a token.
	for _, path := range []string{"/api/v1/me/faves/posts", "/api/v1/me/faves/comments", "/api/v1/me/posts"} {
		if w = newHarness(rest.Config{}).do("GET", path, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a token: %d", path, w.Code)
		}
		if w = newHarness(rest.Config{}).do("GET", path+"?limit=x", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Fatalf("%s with a bad limit: %d", path, w.Code)
		}
	}
}

func TestListOwnPosts(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/me/posts", "", "Authorization", "Bearer tok")
	got := decode[favePostsBody](t, w)

	if w.Code != http.StatusOK || got.Total != 1 || got.NextCursor != "" || len(got.Posts) != 1 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if p := got.Posts[0]; p.ID != "p7" || !p.Mine || p.Wall["id"] != "u2" {
		t.Fatalf("post = %+v", p)
	}
	if h.faves.asked != "own" || h.faves.token != "tok" {
		t.Fatalf("request = %+v", h.faves)
	}
}

func TestListFaveComments(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/me/faves/comments?limit=3", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Comments []struct {
			ID     string         `json:"id"`
			PostID string         `json:"postId"`
			Author map[string]any `json:"author"`
			Text   string         `json:"text"`
			Liked  bool           `json:"liked"`
		} `json:"comments"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)

	// The comment of the deleted page and the one under a post that is
	// gone are left out.
	if w.Code != http.StatusOK || got.Total != 3 || got.NextCursor != "1_c7" || len(got.Comments) != 1 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if c := got.Comments[0]; c.ID != "c5" || c.PostID != "p1" || c.Author["id"] != "u2" || c.Text != "согласен" || !c.Liked {
		t.Fatalf("comment = %+v", c)
	}
	if h.faves.asked != "comments" || h.faves.limit != 3 {
		t.Fatalf("request = %+v", h.faves)
	}
}
