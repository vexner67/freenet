package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Articles are the long reads of the editor, which the post service
// keeps; anybody reads them, their authors change them.
type Articles interface {
	Create(ctx context.Context, accessToken string, d model.ArticleDraft) (model.Article, error)
	Edit(ctx context.Context, accessToken, id string, d model.ArticleDraft) (model.Article, error)
	Delete(ctx context.Context, accessToken, id string) error
	Get(ctx context.Context, id string, brief bool) (model.Article, error)
	List(ctx context.Context, authorID, cursor string, limit int) (model.ArticlePage, error)
}

type articleSpanJSON struct {
	Text   string `json:"text"`
	Bold   bool   `json:"bold,omitempty"`
	Italic bool   `json:"italic,omitempty"`
	Strike bool   `json:"strike,omitempty"`
	Link   string `json:"link,omitempty"`
}

// articleBlockJSON is a block of an article; photo is the ID of a photo
// of the author in a request and the photo in an answer.
type articleBlockJSON struct {
	Kind    string            `json:"kind"`
	Spans   []articleSpanJSON `json:"spans,omitempty"`
	Photo   *photoJSON        `json:"photo,omitempty"`
	Caption string            `json:"caption,omitempty"`
}

type articleJSON struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Cover  *photoJSON `json:"cover"`
	Author authorJSON `json:"author"`
	// Blocks are empty in a brief article: a card or a list.
	Blocks    []articleBlockJSON `json:"blocks"`
	Excerpt   string             `json:"excerpt"`
	Minutes   int                `json:"minutes"`
	CreatedAt string             `json:"createdAt"`
	EditedAt  string             `json:"editedAt,omitempty"`
}

type articlePageResponse struct {
	Articles   []articleJSON `json:"articles"`
	NextCursor string        `json:"nextCursor"`
	Total      int           `json:"total"`
}

type articleBlockRequest struct {
	Kind    string            `json:"kind"`
	Spans   []articleSpanJSON `json:"spans"`
	Photo   string            `json:"photo"`
	Caption string            `json:"caption"`
}

type articleRequest struct {
	Title  string                `json:"title"`
	Cover  string                `json:"cover"`
	Blocks []articleBlockRequest `json:"blocks"`
}

func (req articleRequest) draft() model.ArticleDraft {
	d := model.ArticleDraft{Title: req.Title, CoverID: req.Cover, Blocks: make([]model.ArticleBlock, 0, len(req.Blocks))}
	for _, rb := range req.Blocks {
		b := model.ArticleBlock{Kind: rb.Kind, PhotoID: rb.Photo, Caption: rb.Caption}
		for _, s := range rb.Spans {
			b.Spans = append(b.Spans, model.ArticleSpan(s))
		}
		d.Blocks = append(d.Blocks, b)
	}

	return d
}

func toArticleJSON(a model.Article, author model.Author) articleJSON {
	resp := articleJSON{
		ID: a.ID, Title: a.Title, Author: toAuthorJSON(author), Blocks: []articleBlockJSON{},
		Excerpt: a.Excerpt, Minutes: a.Minutes, CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
	}
	if a.CoverID != "" {
		cover := toPhotoJSON(a.AuthorID, a.CoverID)
		resp.Cover = &cover
	}
	if a.EditedAt != nil {
		resp.EditedAt = a.EditedAt.UTC().Format(time.RFC3339)
	}

	for _, b := range a.Blocks {
		jb := articleBlockJSON{Kind: b.Kind, Caption: b.Caption}
		if b.PhotoID != "" {
			photo := toPhotoJSON(a.AuthorID, b.PhotoID)
			jb.Photo = &photo
		}
		for _, s := range b.Spans {
			jb.Spans = append(jb.Spans, articleSpanJSON(s))
		}
		resp.Blocks = append(resp.Blocks, jb)
	}

	return resp
}

// createArticle writes an article of the caller, whose profile is filled
// in, with photos they uploaded.
func (a *api) createArticle(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req articleRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before writing articles")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	d := req.draft()
	if err = a.claimPhotos(r, token, d.Photos()); err != nil {
		a.writeError(w, r, err)
		return
	}

	created, err := a.articles.Create(r.Context(), token, d)
	if err != nil {
		a.releasePhotos(r, token, d.Photos(), err)
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toArticleJSON(created, meAuthor(me)))
}

// editArticle writes an article of the caller anew; the photos it did not
// show before are claimed.
func (a *api) editArticle(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req articleRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	old, err := a.articles.Get(r.Context(), r.PathValue("id"), false)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if old.AuthorID != me.ID {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "only the author can change the article"))
		return
	}

	d := req.draft()
	added := without(d.Photos(), old.Photos())
	if err = a.claimPhotos(r, token, added); err != nil {
		a.writeError(w, r, err)
		return
	}

	edited, err := a.articles.Edit(r.Context(), token, old.ID, d)
	if err != nil {
		a.releasePhotos(r, token, added, err)
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toArticleJSON(edited, meAuthor(me)))
}

func (a *api) deleteArticle(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.articles.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getArticle reads an article with its author, in full or brief
// (?brief=true, for the card of a post); that of a deleted page is not
// found.
func (a *api) getArticle(w http.ResponseWriter, r *http.Request) {
	art, err := a.articles.Get(r.Context(), r.PathValue("id"), r.URL.Query().Get("brief") == "true")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.people(r, []string{art.AuthorID})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	author, ok := people[art.AuthorID]
	if !ok || author.Deactivated {
		a.writeError(w, r, failure.New(failure.NotFound, "article not found"))
		return
	}

	writeJSON(w, http.StatusOK, toArticleJSON(art, author))
}

// listArticles pages the articles of a person, newest first, brief.
func (a *api) listArticles(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	profile, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.articles.List(r.Context(), profile.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	author := authorOfProfile(profile)
	resp := articlePageResponse{Articles: []articleJSON{}, NextCursor: page.NextCursor, Total: page.Total}
	for _, art := range page.Articles {
		resp.Articles = append(resp.Articles, toArticleJSON(art, author))
	}

	writeJSON(w, http.StatusOK, resp)
}
