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

// fakeArticles knows a1, Ann's (u1) article with the cover ph1, and a3 of
// Cat (u3), whose page is deleted.
type fakeArticles struct {
	token, deleted, author string
	brief                  bool
	drafts                 []model.ArticleDraft
	err                    error
}

func (f *fakeArticles) known(id string) (model.Article, bool) {
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	switch id {
	case "a1":
		return model.Article{ID: "a1", AuthorID: "u1", Minutes: 2, Excerpt: "Начало", CreatedAt: at, ArticleDraft: model.ArticleDraft{
			Title: "Статья", CoverID: "ph1", Blocks: []model.ArticleBlock{
				{Kind: "heading", Spans: []model.ArticleSpan{{Text: "Глава"}}},
				{Kind: "paragraph", Spans: []model.ArticleSpan{{Text: "Текст", Bold: true, Link: "https://example.com"}}},
			},
		}}, true
	case "a3":
		return model.Article{ID: "a3", AuthorID: "u3", CreatedAt: at, ArticleDraft: model.ArticleDraft{Title: "Её"}}, true
	}

	return model.Article{}, false
}

func (f *fakeArticles) Create(_ context.Context, token string, d model.ArticleDraft) (model.Article, error) {
	f.token, f.drafts = token, append(f.drafts, d)
	return model.Article{ID: "a9", AuthorID: "u1", ArticleDraft: d}, f.err
}

func (f *fakeArticles) Edit(_ context.Context, token, id string, d model.ArticleDraft) (model.Article, error) {
	f.token, f.drafts = token, append(f.drafts, d)
	return model.Article{ID: id, AuthorID: "u1", ArticleDraft: d}, f.err
}

func (f *fakeArticles) Delete(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

func (f *fakeArticles) Get(_ context.Context, id string, brief bool) (model.Article, error) {
	f.brief = brief
	a, ok := f.known(id)
	if !ok {
		return model.Article{}, failure.New(failure.NotFound, "article not found")
	}
	if brief {
		a.Blocks = nil
	}
	return a, nil
}

func (f *fakeArticles) List(_ context.Context, authorID, _ string, _ int) (model.ArticlePage, error) {
	f.author = authorID
	a, _ := f.known("a1")
	a.Blocks = nil
	return model.ArticlePage{Articles: []model.Article{a}, Total: 1}, f.err
}

type articleBody struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Cover *struct {
		Large string `json:"large"`
	} `json:"cover"`
	Author struct {
		Nickname string `json:"nickname"`
	} `json:"author"`
	Blocks []struct {
		Kind  string `json:"kind"`
		Spans []struct {
			Text string `json:"text"`
			Bold bool   `json:"bold"`
			Link string `json:"link"`
		} `json:"spans"`
		Photo *struct {
			ID string `json:"id"`
		} `json:"photo"`
	} `json:"blocks"`
	Minutes int `json:"minutes"`
}

func TestGetArticle(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/articles/a1", "")
	got := decode[articleBody](t, w)
	if w.Code != http.StatusOK || got.Title != "Статья" || got.Author.Nickname != "ann" || got.Minutes != 2 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if got.Cover == nil || got.Cover.Large != "/media/photos/u1/ph1/large.jpg" {
		t.Fatalf("cover = %+v", got.Cover)
	}
	if len(got.Blocks) != 2 || got.Blocks[1].Spans[0].Link != "https://example.com" || !got.Blocks[1].Spans[0].Bold {
		t.Fatalf("blocks = %+v", got.Blocks)
	}

	if w = h.do("GET", "/api/v1/articles/a1?brief=true", ""); w.Code != http.StatusOK || !h.articles.brief || len(decode[articleBody](t, w).Blocks) != 0 {
		t.Fatalf("brief: %d %s", w.Code, w.Body)
	}

	// A deleted page has no articles; nor is there one that is not.
	for _, id := range []string{"a3", "a0"} {
		if w = h.do("GET", "/api/v1/articles/"+id, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", id, w.Code)
		}
	}

	w = h.do("GET", "/api/v1/users/ann/articles", "")
	if w.Code != http.StatusOK || h.articles.author != "u1" {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
}

func TestCreateArticle(t *testing.T) {
	t.Parallel()

	body := `{"title":"Новая","cover":"ph1","blocks":[{"kind":"paragraph","spans":[{"text":"Привет","italic":true}]},{"kind":"photo","photo":"ph2","caption":"Вид"}]}`

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	w := h.do("POST", "/api/v1/articles", body, "Authorization", "Bearer tok")
	got := decode[articleBody](t, w)
	if w.Code != http.StatusCreated || got.ID != "a9" || got.Author.Nickname != "ann" || len(got.Blocks) != 2 || got.Blocks[1].Photo.ID != "ph2" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if d := h.articles.drafts[0]; d.Title != "Новая" || !d.Blocks[0].Spans[0].Italic || !slices.Equal(d.Photos(), []string{"ph1", "ph2"}) {
		t.Fatalf("draft = %+v", d)
	}
	if !slices.Equal(h.media.claimed, []string{"ph1,ph2"}) {
		t.Fatalf("claimed = %v", h.media.claimed)
	}

	// Refused, the photos go back.
	h = newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	h.articles.err = failure.New(failure.InvalidArgument, "article title is empty")
	if w = h.do("POST", "/api/v1/articles", body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || !slices.Equal(h.media.released, []string{"ph1,ph2"}) {
		t.Fatalf("refused: %d, released %v", w.Code, h.media.released)
	}

	// Not one's own photo; an empty profile; nobody.
	h = newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	if w = h.do("POST", "/api/v1/articles", `{"title":"Т","cover":"ph7"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || len(h.articles.drafts) != 0 {
		t.Fatalf("another's photo: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/articles", body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("empty profile: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/articles", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestEditArticle(t *testing.T) {
	t.Parallel()

	// The cover is the article's already; only the new photo is claimed.
	h := newHarness(rest.Config{})
	body := `{"title":"Другая","cover":"ph1","blocks":[{"kind":"paragraph","spans":[{"text":"x"}]},{"kind":"photo","photo":"ph2"}]}`
	w := h.do("PUT", "/api/v1/articles/a1", body, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || decode[articleBody](t, w).Title != "Другая" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !slices.Equal(h.media.claimed, []string{"ph2"}) {
		t.Fatalf("claimed = %v", h.media.claimed)
	}

	// Another's article is not edited, nor its photos claimed.
	h = newHarness(rest.Config{})
	if w = h.do("PUT", "/api/v1/articles/a3", body, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden || len(h.media.claimed) != 0 {
		t.Fatalf("another's: %d %v", w.Code, h.media.claimed)
	}

	if w = h.do("DELETE", "/api/v1/articles/a1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.articles.deleted != "a1" {
		t.Fatalf("delete: %d", w.Code)
	}
}

// A post that links to an article of the site gets no card of a preview:
// the site draws the article's card itself.
func TestOwnArticleLinkHasNoPreview(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	w := h.do("POST", "/api/v1/me/posts", `{"text":"Читайте http://example.com/article/a1"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusCreated || len(h.previews.read) != 0 {
		t.Fatalf("%d, read %v", w.Code, h.previews.read)
	}
}
