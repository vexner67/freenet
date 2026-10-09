package rest_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type wall struct {
	Posts []struct {
		ID       string            `json:"id"`
		Text     string            `json:"text"`
		Author   map[string]string `json:"author"`
		Wall     map[string]string `json:"wall"`
		Likes    int               `json:"likes"`
		Liked    bool              `json:"liked"`
		Comments int               `json:"comments"`
	} `json:"posts"`
	NextCursor string `json:"nextCursor"`
	Total      int    `json:"total"`
	CanPost    bool   `json:"canPost"`
}

func TestListPosts(t *testing.T) {
	t.Parallel()

	t.Run("wall of a nickname with the authors of its posts", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("GET", "/api/v1/users/ann/posts?cursor=p5&limit=2", "")
		got := decode[wall](t, w)

		if w.Code != http.StatusOK || got.Total != 7 || got.NextCursor != "p1" || len(got.Posts) != 3 {
			t.Fatalf("%d %+v", w.Code, got)
		}
		if p := got.Posts[0]; p.Author["nickname"] != "bob" || p.Wall["nickname"] != "ann" {
			t.Fatalf("bob's post on ann's wall: author = %v, wall = %v", p.Author, p.Wall)
		}
		if p := got.Posts[1]; p.Author["nickname"] != "ann" || p.Author["name"] != "Ann" || p.Author["id"] != "u1" || p.Wall != nil {
			t.Fatalf("ann's own post: author = %v, wall = %v (want null)", p.Author, p.Wall)
		}
		if h.posts.cursor != "p5" || h.posts.limit != 2 || h.posts.token != "" {
			t.Fatalf("cursor = %q, limit = %d, token = %q", h.posts.cursor, h.posts.limit, h.posts.token)
		}
		if p := got.Posts[2]; p.Likes != 2 || !p.Liked || got.Posts[1].Comments != 4 {
			t.Fatalf("counters: %+v", got.Posts)
		}
		if got.CanPost || h.users.wallAsked != "" {
			t.Fatalf("anonymous viewer: canPost = %v, asked about %q", got.CanPost, h.users.wallAsked)
		}
	})

	t.Run("a signed-in viewer's token goes along for likes and canPost", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("GET", "/api/v1/users/ann/posts", "", "Authorization", "Bearer tok")
		if got := decode[wall](t, w); w.Code != http.StatusOK || h.posts.token != "tok" || !got.CanPost || h.users.wallAsked != "u1" {
			t.Fatalf("status = %d, token = %q, canPost = %v", w.Code, h.posts.token, got.CanPost)
		}

		closed := newHarness(rest.Config{})
		closed.users.closedWall = true
		if got := decode[wall](t, closed.do("GET", "/api/v1/users/ann/posts", "", "Authorization", "Bearer tok")); got.CanPost {
			t.Fatal("canPost on a closed wall")
		}

		next := newHarness(rest.Config{})
		if got := decode[wall](t, next.do("GET", "/api/v1/users/ann/posts?cursor=p1", "", "Authorization", "Bearer tok")); got.CanPost || next.users.wallAsked != "" {
			t.Fatal("canPost is told on the first page only")
		}
	})

	t.Run("unknown nickname", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("GET", "/api/v1/users/nobody/posts", ""); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	for _, limit := range []string{"0", "51", "ten"} {
		t.Run("limit "+limit, func(t *testing.T) {
			t.Parallel()

			if w := newHarness(rest.Config{}).do("GET", "/api/v1/users/ann/posts?limit="+limit, ""); w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", w.Code)
			}
		})
	}
}

func TestCreatePost(t *testing.T) {
	t.Parallel()

	t.Run("on the caller's wall", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann", Gender: "female"}

		w := h.do("POST", "/api/v1/me/posts", `{"text":"привет"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if w.Code != http.StatusCreated || h.posts.created != "привет" || h.posts.token != "tok" {
			t.Fatalf("%d %v", w.Code, got)
		}
		if author, _ := got["author"].(map[string]any); author["nickname"] != "ann" || author["gender"] != "female" {
			t.Fatalf("author = %v", got["author"])
		}
		if got["createdAt"] != "2026-10-04T15:30:00Z" {
			t.Fatalf("createdAt = %v", got["createdAt"])
		}
	})

	t.Run("profile must be filled in first", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/me/posts", `{"text":"привет"}`, "Authorization", "Bearer tok")

		if w.Code != http.StatusBadRequest || h.posts.created != "" {
			t.Fatalf("status = %d, created = %q", w.Code, h.posts.created)
		}
	})

	t.Run("needs a token", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("POST", "/api/v1/me/posts", `{"text":"x"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestCreateWallPost(t *testing.T) {
	t.Parallel()

	newAnn := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann", Gender: "female"}
		return h
	}

	t.Run("on someone else's wall", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		w := h.do("POST", "/api/v1/users/bob/posts", `{"text":"привет, Боб"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if w.Code != http.StatusCreated || h.posts.wall != "u2" || h.users.wallAsked != "u2" || h.posts.created != "привет, Боб" {
			t.Fatalf("%d %v; wall = %q, asked = %q", w.Code, got, h.posts.wall, h.users.wallAsked)
		}
		if author, _ := got["author"].(map[string]any); author["nickname"] != "ann" {
			t.Fatalf("author = %v", got["author"])
		}
		if wall, _ := got["wall"].(map[string]any); wall["nickname"] != "bob" {
			t.Fatalf("wall = %v", got["wall"])
		}
	})

	t.Run("on the caller's own wall by nickname", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		w := h.do("POST", "/api/v1/users/ann/posts", `{"text":"своё"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if w.Code != http.StatusCreated || h.posts.wall != "" || h.users.wallAsked != "" || got["wall"] != nil {
			t.Fatalf("%d %v; wall = %q, asked = %q", w.Code, got, h.posts.wall, h.users.wallAsked)
		}
	})

	t.Run("a closed wall", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		h.users.closedWall = true
		w := h.do("POST", "/api/v1/users/bob/posts", `{"text":"x"}`, "Authorization", "Bearer tok")

		got := decode[map[string]map[string]string](t, w)
		if w.Code != http.StatusForbidden || got["error"]["message"] != "you cannot post on this wall" || h.posts.created != "" {
			t.Fatalf("%d %v", w.Code, got)
		}
	})

	t.Run("an unknown wall", func(t *testing.T) {
		t.Parallel()

		if w := newAnn().do("POST", "/api/v1/users/nobody/posts", `{"text":"x"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("profile must be filled in first", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		if w := h.do("POST", "/api/v1/users/bob/posts", `{"text":"x"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.posts.created != "" {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestDeletePost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("DELETE", "/api/v1/posts/p1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.deleted != "p1" {
		t.Fatalf("status = %d, deleted = %q", w.Code, h.posts.deleted)
	}

	h = newHarness(rest.Config{})
	h.posts.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("DELETE", "/api/v1/posts/p1", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("someone else's post: status = %d", w.Code)
	}

	if w := newHarness(rest.Config{}).do("DELETE", "/api/v1/posts/p404", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("missing post: status = %d", w.Code)
	}

	if w := newHarness(rest.Config{}).do("DELETE", "/api/v1/posts/p1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: status = %d", w.Code)
	}
}

func TestGetPost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/posts/p1", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)

	if w.Code != http.StatusOK || got["text"] != "первая" || got["likes"] != 3.0 || got["liked"] != true || got["comments"] != 2.0 {
		t.Fatalf("%d %v", w.Code, got)
	}
	if author, _ := got["author"].(map[string]any); author["nickname"] != "ann" {
		t.Fatalf("author = %v", got["author"])
	}
	if h.posts.token != "tok" {
		t.Fatalf("token = %q", h.posts.token)
	}

	anonymous := newHarness(rest.Config{})
	if w = anonymous.do("GET", "/api/v1/posts/p1", ""); w.Code != http.StatusOK || decode[map[string]any](t, w)["liked"] != false {
		t.Fatalf("anonymous: %d", w.Code)
	}

	// The wall of an author who deleted the account is gone with it.
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/posts/p2", ""); w.Code != http.StatusNotFound {
		t.Fatalf("author gone: status = %d", w.Code)
	}

	guest := decode[map[string]any](t, newHarness(rest.Config{}).do("GET", "/api/v1/posts/p4", ""))
	if author, _ := guest["author"].(map[string]any); author["nickname"] != "bob" {
		t.Fatalf("author of a post on someone else's wall = %v", guest["author"])
	}
	if wall, _ := guest["wall"].(map[string]any); wall["nickname"] != "ann" {
		t.Fatalf("wall = %v", guest["wall"])
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/posts/p404", ""); w.Code != http.StatusNotFound {
		t.Fatalf("missing post: status = %d", w.Code)
	}
}

func TestLikePost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("PUT", "/api/v1/posts/p1/like", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["likes"] != 1.0 || got["liked"] != true {
		t.Fatalf("like: %d %v", w.Code, got)
	}
	if h.posts.token != "tok" || !h.posts.liked["p1"] {
		t.Fatalf("token = %q, liked = %v", h.posts.token, h.posts.liked)
	}

	w = h.do("DELETE", "/api/v1/posts/p1/like", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["likes"] != 0.0 || got["liked"] != false {
		t.Fatalf("unlike: %d %v", w.Code, got)
	}

	if w = newHarness(rest.Config{}).do("PUT", "/api/v1/posts/p1/like", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: status = %d", w.Code)
	}

	gone := newHarness(rest.Config{})
	gone.posts.err = failure.New(failure.NotFound, "post not found")
	if w = gone.do("PUT", "/api/v1/posts/p1/like", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("missing post: status = %d", w.Code)
	}
}

func TestRepostPost(t *testing.T) {
	t.Parallel()

	t.Run("shares a post and shows it inside", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/posts/p1/reposts", `{"text":"смотрите"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if w.Code != http.StatusCreated || h.posts.shared != "p1" || h.posts.created != "смотрите" || h.posts.token != "tok" {
			t.Fatalf("%d %v", w.Code, got)
		}
		if got["text"] != "смотрите" || got["wall"] != nil {
			t.Fatalf("repost = %v", got)
		}

		repost, _ := got["repost"].(map[string]any)
		author, _ := repost["author"].(map[string]any)
		photos, _ := repost["photos"].([]any)
		if repost["id"] != "p1" || repost["deleted"] != false || repost["text"] != "первая" || author["nickname"] != "ann" || len(photos) != 1 {
			t.Fatalf("shared post = %v", repost)
		}
		if _, hasCounters := repost["likes"]; hasCounters {
			t.Fatalf("a shared post is shown without its counters: %v", repost)
		}
	})

	t.Run("a missing post", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		if w := h.do("POST", "/api/v1/posts/p404/reposts", `{}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("profile must be filled in first", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		if w := h.do("POST", "/api/v1/posts/p1/reposts", `{}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.posts.shared != "" {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("needs a token", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("POST", "/api/v1/posts/p1/reposts", `{}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("a repost of a deleted post", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("GET", "/api/v1/posts/p8", "")
		got := decode[map[string]any](t, w)

		repost, _ := got["repost"].(map[string]any)
		if w.Code != http.StatusOK || repost["id"] != "p-deleted" || repost["deleted"] != true || repost["author"] != nil {
			t.Fatalf("%d %v", w.Code, got)
		}
	})

	t.Run("plain posts have no repost and count theirs", func(t *testing.T) {
		t.Parallel()

		got := decode[map[string]any](t, newHarness(rest.Config{}).do("GET", "/api/v1/posts/p1", ""))
		if got["repost"] != nil || got["reposts"] != 0.0 {
			t.Fatalf("plain post = %v", got)
		}
	})
}

func TestListPosts_ManyPeople(t *testing.T) {
	t.Parallel()

	// 60 guests and their walls: more people than one GetProfiles takes.
	h := newHarness(rest.Config{})
	for i := range 60 {
		h.posts.wallPosts = append(h.posts.wallPosts, model.Post{
			ID:     fmt.Sprintf("p%d", i),
			Author: model.Author{ID: fmt.Sprintf("xa%d", i)},
			Wall:   model.Author{ID: fmt.Sprintf("xw%d", i)},
			Text:   "привет",
		})
	}

	w := h.do("GET", "/api/v1/users/ann/posts", "")
	if got := decode[wall](t, w); w.Code != http.StatusOK || len(got.Posts) != 60 || h.users.profileCalls != 2 {
		t.Fatalf("%d: %d posts in %d GetProfiles", w.Code, len(got.Posts), h.users.profileCalls)
	}
}

func TestEditPost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("PATCH", "/api/v1/posts/p1", `{"text":"по-новому","photos":["ph2"]}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.posts.edited != "по-новому" || len(h.posts.photos) != 1 {
		t.Fatalf("status = %d, edited %q with %v: %s", w.Code, h.posts.edited, h.posts.photos, w.Body)
	}

	var p struct {
		Text     string  `json:"text"`
		EditedAt *string `json:"editedAt"`
		Author   struct {
			Nickname string `json:"nickname"`
		} `json:"author"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Text != "по-новому" || p.EditedAt == nil || *p.EditedAt != "2026-10-05T09:00:00Z" || p.Author.Nickname == "" {
		t.Fatalf("edited post = %+v", p)
	}

	for _, tt := range []struct {
		name, path, body string
		headers          []string
		want             int
	}{
		{"someone else's photo", "/api/v1/posts/p1", `{"text":"x","photos":["ph7"]}`, []string{"Authorization", "Bearer tok"}, http.StatusBadRequest},
		{"a missing post", "/api/v1/posts/p404", `{"text":"x"}`, []string{"Authorization", "Bearer tok"}, http.StatusNotFound},
		{"no token", "/api/v1/posts/p1", `{"text":"x"}`, nil, http.StatusUnauthorized},
	} {
		if w := newHarness(rest.Config{}).do("PATCH", tt.path, tt.body, tt.headers...); w.Code != tt.want {
			t.Errorf("%s: status = %d, want %d", tt.name, w.Code, tt.want)
		}
	}

	h = newHarness(rest.Config{})
	h.posts.err = failure.New(failure.Conflict, "the post can no longer be edited")
	if w := h.do("PATCH", "/api/v1/posts/p1", `{"text":"поздно"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("an old post: status = %d", w.Code)
	}
}

func TestArchivePostAndCloseComments(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	for _, path := range []string{"/api/v1/posts/p4/archive", "/api/v1/posts/p4/closed-comments"} {
		if w := h.do("PUT", path, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a token: %d", path, w.Code)
		}
	}

	if w := h.do("PUT", "/api/v1/posts/p4/archive", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.archived != "p4" {
		t.Fatalf("archive: %d, %q", w.Code, h.posts.archived)
	}
	if w := h.do("DELETE", "/api/v1/posts/p4/archive", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.archived != "-p4" {
		t.Fatalf("unarchive: %d, %q", w.Code, h.posts.archived)
	}
	if w := h.do("PUT", "/api/v1/posts/p4/closed-comments", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.closed != "p4" {
		t.Fatalf("close: %d, %q", w.Code, h.posts.closed)
	}
	if w := h.do("DELETE", "/api/v1/posts/p4/closed-comments", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.closed != "-p4" {
		t.Fatalf("open: %d, %q", w.Code, h.posts.closed)
	}
	if w := h.do("PUT", "/api/v1/posts/p404/archive", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("no post: %d", w.Code)
	}

	h.posts.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("PUT", "/api/v1/posts/p1/archive", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("someone else's wall: %d", w.Code)
	}
}

func TestPinPost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/posts/p4/pin", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.pinned != "p4" {
		t.Fatalf("pin: status = %d, pinned %q", w.Code, h.posts.pinned)
	}
	if w := h.do("DELETE", "/api/v1/posts/p4/pin", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.pinned != "-p4" {
		t.Fatalf("unpin: status = %d, pinned %q", w.Code, h.posts.pinned)
	}

	h = newHarness(rest.Config{})
	h.posts.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("PUT", "/api/v1/posts/p1/pin", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("someone else's wall: status = %d", w.Code)
	}
	if w := newHarness(rest.Config{}).do("PUT", "/api/v1/posts/p404/pin", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("a missing post: status = %d", w.Code)
	}
	if w := newHarness(rest.Config{}).do("PUT", "/api/v1/posts/p1/pin", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d", w.Code)
	}
}

func TestRepostInClub(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	// "в группу": anyone on an open wall, on their own behalf.
	w := h.do("POST", "/api/v1/posts/p1/reposts", `{"text":"смотрите","club":"cats"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusCreated || h.posts.repost != (model.NewRepost{Text: "смотрите", WallID: "k1"}) {
		t.Fatalf("%d %s, asked %+v", w.Code, w.Body, h.posts.repost)
	}

	// For the club only its admins; on a closed wall always for it.
	if w = h.do("POST", "/api/v1/posts/p1/reposts", `{"club":"cats","fromClub":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a member for the club: %d", w.Code)
	}
	h.clubs.role, h.clubs.closedWall = "admin", true
	w = h.do("POST", "/api/v1/posts/p1/reposts", `{"club":"cats","signed":true}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusCreated || h.posts.repost != (model.NewRepost{WallID: "k1", FromWall: true, Signed: true}) {
		t.Fatalf("an admin on a closed wall: %d, asked %+v", w.Code, h.posts.repost)
	}

	if w = h.do("POST", "/api/v1/posts/p1/reposts", `{"club":"nobody"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("no such club: %d", w.Code)
	}
}
