package rest_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

var storyTime = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// fakeStories tells of Ann (u1), Bob (u2) and Cat (u3, deleted): Bob's
// first story the viewer viewed, his second not.
type fakeStories struct {
	token, told, text, viewed, deleted, of, cursor string
	liked, unliked, commented, uncommented         string
	hidden, ignored                                []string
	unseen                                         bool
	err                                            error
}

func (f *fakeStories) stories() []model.Story {
	story := func(id, author string, viewed bool) model.Story {
		return model.Story{
			ID: id, AuthorID: author, PhotoID: "ph-" + id, Text: id, CreatedAt: storyTime, ExpiresAt: storyTime.Add(24 * time.Hour),
			Viewed: viewed, Mine: author == "u1",
		}
	}

	return []model.Story{story("s1", "u1", true), story("s2", "u2", true), story("s3", "u2", false), story("s4", "u3", false)}
}

func (f *fakeStories) Tell(_ context.Context, token, photoID, text string) (model.Story, error) {
	f.token, f.told, f.text = token, photoID, text
	return model.Story{ID: "s9", AuthorID: "u1", PhotoID: photoID, Text: text, CreatedAt: storyTime, Mine: true}, f.err
}

func (f *fakeStories) Feed(_ context.Context, token string, hidden, ignored []string) ([]model.Story, error) {
	f.token, f.hidden, f.ignored = token, hidden, ignored
	return f.stories(), f.err
}

func (f *fakeStories) User(_ context.Context, token, userID string, hidden []string) ([]model.Story, error) {
	f.token, f.of, f.hidden = token, userID, hidden
	return slices.DeleteFunc(f.stories(), func(s model.Story) bool { return s.AuthorID != userID }), f.err
}

func (f *fakeStories) View(_ context.Context, token, id string, hidden bool) error {
	f.token, f.viewed, f.unseen = token, id, hidden
	if id != "s2" {
		return failure.New(failure.NotFound, "story not found")
	}
	return f.err
}

func (f *fakeStories) Viewers(_ context.Context, token, _, cursor string, _ int) (model.LikerPage, error) {
	f.token, f.cursor = token, cursor
	return model.LikerPage{UserIDs: []string{"u2", "u3"}, NextCursor: "next", Total: 2}, f.err
}

func (f *fakeStories) Delete(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

func (f *fakeStories) Get(_ context.Context, token, id string) (model.Story, error) {
	f.token = token
	for _, s := range f.stories() {
		if s.ID == id {
			return s, f.err
		}
	}
	return model.Story{}, failure.New(failure.NotFound, "story not found")
}

func (f *fakeStories) Like(_ context.Context, token, id string) error {
	f.token, f.liked = token, id
	return f.err
}

func (f *fakeStories) Unlike(_ context.Context, token, id string) error {
	f.token, f.unliked = token, id
	return f.err
}

func (f *fakeStories) Comment(_ context.Context, token, id, text string) (model.StoryComment, error) {
	f.token, f.commented = token, id+"/"+text
	return model.StoryComment{ID: "sc9", StoryID: id, AuthorID: "u1", Text: text, CreatedAt: storyTime, Mine: true}, f.err
}

func (f *fakeStories) Comments(_ context.Context, token, id, cursor string, _ int) (model.StoryCommentPage, error) {
	f.token, f.cursor = token, cursor
	comment := func(cid, author string) model.StoryComment {
		return model.StoryComment{ID: cid, StoryID: id, AuthorID: author, Text: cid, CreatedAt: storyTime, Mine: author == "u1"}
	}
	return model.StoryCommentPage{Comments: []model.StoryComment{comment("sc1", "u2"), comment("sc2", "u3")}, NextCursor: "sc2", Total: 2}, f.err
}

func (f *fakeStories) DeleteComment(_ context.Context, token, id string) error {
	f.token, f.uncommented = token, id
	return f.err
}

type storiesBody struct {
	Authors []struct {
		Author  map[string]any `json:"author"`
		Viewed  bool           `json:"viewed"`
		Stories []struct {
			ID    string            `json:"id"`
			Photo map[string]string `json:"photo"`
			Mine  bool              `json:"mine"`
		} `json:"stories"`
	} `json:"authors"`
}

func TestListFeedStories(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.blockedIDs, h.users.ignoredIDs = []string{"u7"}, []string{"u8"}

	w := h.do("GET", "/api/v1/stories", "", "Authorization", "Bearer tok")
	got := decode[storiesBody](t, w)
	// Cat's page is deleted: her story is left out.
	if w.Code != http.StatusOK || len(got.Authors) != 2 || got.Authors[0].Author["nickname"] != "ann" || !got.Authors[0].Stories[0].Mine ||
		got.Authors[1].Author["nickname"] != "bob" || len(got.Authors[1].Stories) != 2 || got.Authors[1].Viewed ||
		got.Authors[1].Stories[0].Photo["small"] != "/media/photos/u2/ph-s2/small.jpg" {
		t.Fatalf("stories: %d %+v", w.Code, got)
	}
	if h.stories.token != "tok" || !slices.Equal(h.stories.hidden, []string{"u7"}) || !slices.Equal(h.stories.ignored, []string{"u8"}) {
		t.Fatalf("asked with %q, hidden %v, ignored %v", h.stories.token, h.stories.hidden, h.stories.ignored)
	}

	if w = h.do("GET", "/api/v1/stories", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}

func TestListUserStories(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/users/bob/stories", "", "Authorization", "Bearer tok")
	got := decode[storiesBody](t, w)
	if w.Code != http.StatusOK || len(got.Authors) != 1 || len(got.Authors[0].Stories) != 2 || h.stories.of != "u2" {
		t.Fatalf("Bob's stories: %d %+v", w.Code, got)
	}

	if w = h.do("GET", "/api/v1/users/cat/stories", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a deleted page's stories: %d", w.Code)
	}
}

func TestTellStory(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	w := h.do("POST", "/api/v1/me/stories", `{"photo":"ph1","text":"Утро"}`, "Authorization", "Bearer tok")
	got := decode[struct {
		ID    string            `json:"id"`
		Text  string            `json:"text"`
		Photo map[string]string `json:"photo"`
		Mine  bool              `json:"mine"`
	}](t, w)
	if w.Code != http.StatusCreated || got.ID != "s9" || got.Text != "Утро" || !got.Mine || got.Photo["large"] != "/media/photos/u1/ph1/large.jpg" {
		t.Fatalf("tell: %d %+v", w.Code, got)
	}
	if h.stories.told != "ph1" || !slices.Equal(h.media.claimed, []string{"ph1"}) {
		t.Fatalf("told %q, claimed %v", h.stories.told, h.media.claimed)
	}

	// Another's photo is refused before post hears of it.
	h.stories.told = ""
	if w = h.do("POST", "/api/v1/me/stories", `{"photo":"zz"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.stories.told != "" {
		t.Fatalf("another's photo: %d, told %q", w.Code, h.stories.told)
	}

	// A refusal of post gives the photo back.
	h.stories.err = failure.New(failure.Conflict, "too many stories")
	if w = h.do("POST", "/api/v1/me/stories", `{"photo":"ph2"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict ||
		!slices.Equal(h.media.released, []string{"ph2"}) {
		t.Fatalf("refused: %d, released %v", w.Code, h.media.released)
	}

	h.users.profile = model.Profile{}
	if w = h.do("POST", "/api/v1/me/stories", `{"photo":"ph1"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("without a profile: %d", w.Code)
	}
}

func TestStoryViews(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	if w := h.do("PUT", "/api/v1/stories/s2/view", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.stories.viewed != "s2" || h.stories.unseen {
		t.Fatalf("view: %d", w.Code)
	}
	// "Невидимка" of Freenet Prime watches unseen.
	h.users.prime = model.User{PrimeUntil: time.Now().Add(time.Hour), PrimeInvisible: true}
	if w := h.do("PUT", "/api/v1/stories/s2/view", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || !h.stories.unseen {
		t.Fatalf("an unseen view: %d, %v", w.Code, h.stories.unseen)
	}
	h.users.prime = model.User{}
	if w := h.do("PUT", "/api/v1/stories/s0/view", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("view of none: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/stories/s1/viewers?cursor=c", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Users      []map[string]any `json:"users"`
		NextCursor string           `json:"nextCursor"`
		Total      int              `json:"total"`
	}](t, w)
	// Cat's page is deleted: she is left out, but counted.
	if w.Code != http.StatusOK || len(got.Users) != 1 || got.Users[0]["nickname"] != "bob" || got.Total != 2 || got.NextCursor != "next" ||
		h.stories.cursor != "c" {
		t.Fatalf("viewers: %d %+v", w.Code, got)
	}

	if w = h.do("DELETE", "/api/v1/stories/s1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.stories.deleted != "s1" {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestStoryLikes(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/stories/s2/like", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.stories.liked != "s2" {
		t.Fatalf("like: %d, liked %q", w.Code, h.stories.liked)
	}
	if w := h.do("DELETE", "/api/v1/stories/s2/like", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.stories.unliked != "s2" {
		t.Fatalf("unlike: %d, unliked %q", w.Code, h.stories.unliked)
	}
	if w := h.do("PUT", "/api/v1/stories/s2/like", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("a guest likes: %d", w.Code)
	}
}

func TestStoryComments(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/stories/s2/comments?cursor=c", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Comments []struct {
			ID     string         `json:"id"`
			Author map[string]any `json:"author"`
		} `json:"comments"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)
	// Cat's page is deleted: her comment is left out.
	if w.Code != http.StatusOK || len(got.Comments) != 1 || got.Comments[0].Author["nickname"] != "bob" || got.NextCursor != "sc2" || got.Total != 2 || h.stories.cursor != "c" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	completed := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		return h
	}

	// Under her own story nobody asks about a blacklist.
	h = completed()
	if w = h.do("POST", "/api/v1/stories/s1/comments", `{"text":"мур"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated ||
		h.stories.commented != "s1/мур" || h.users.relationWith != "" {
		t.Fatalf("under her own story: %d %s, relation with %q", w.Code, w.Body, h.users.relationWith)
	}

	h = completed()
	if w = h.do("POST", "/api/v1/stories/s2/comments", `{"text":"мур"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || h.users.relationWith != "u2" {
		t.Fatalf("under the story of another: %d, relation with %q", w.Code, h.users.relationWith)
	}

	h = completed()
	h.users.relation = model.Relation{BlockedMe: true}
	w = h.do("POST", "/api/v1/stories/s2/comments", `{"text":"мур"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != "you cannot comment on this story" || h.stories.commented != "" {
		t.Fatalf("blacklisted: %d %s", w.Code, w.Body)
	}

	h = completed()
	if w = h.do("POST", "/api/v1/stories/nope/comments", `{"text":"мур"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("no story: %d", w.Code)
	}

	h = newHarness(rest.Config{})
	w = h.do("POST", "/api/v1/stories/s2/comments", `{"text":"мур"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusBadRequest || decode[errorBody](t, w).Error.Message != "fill in your profile before commenting" {
		t.Fatalf("without a profile: %d %s", w.Code, w.Body)
	}

	if w = h.do("DELETE", "/api/v1/story-comments/sc1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.stories.uncommented != "sc1" {
		t.Fatalf("delete: %d, deleted %q", w.Code, h.stories.uncommented)
	}
}
