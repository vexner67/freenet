package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type suggestRequest struct {
	Text   string   `json:"text"`
	Photos []string `json:"photos"`
}

// publishRequest is openvk's "Публикация предложенной записи": the text
// as the admin left it and "Подпись автора".
type publishRequest struct {
	Text   string `json:"text"`
	Signed bool   `json:"signed"`
}

// suggestionJSON is a suggested post: its author and what it says.
type suggestionJSON struct {
	ID     string     `json:"id"`
	Author authorJSON `json:"author"`
	Text   string     `json:"text"`
	// TextCut: text is only its beginning, as lists carry it; the whole
	// is at /suggestions/{id}/text.
	TextCut   bool        `json:"textCut,omitempty"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
}

type suggestionsResponse struct {
	Suggestions []suggestionJSON `json:"suggestions"`
	NextCursor  string           `json:"nextCursor"`
	Total       int              `json:"total"`
}

func toSuggestionJSON(s model.Suggestion, author model.Author) suggestionJSON {
	return suggestionJSON{
		ID:        s.ID,
		Author:    toAuthorJSON(author),
		Text:      s.Text,
		TextCut:   s.TextCut,
		Photos:    toPhotosJSON(s.AuthorID, s.PhotoIDs),
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// suggestClubPost is openvk's "Предложить новость": a post for the admins
// of a club whose wall is a "Предложка" to publish.
func (a *api) suggestClubPost(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req suggestRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before posting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	access, err := a.clubs.CanPostOnWall(r.Context(), token, c.Club.ID, false)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !access.Suggest {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "you cannot suggest posts here"))
		return
	}

	if err = a.claimPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	s, err := a.posts.Suggest(r.Context(), token, c.Club.ID, req.Text, req.Photos)
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toSuggestionJSON(s, authorOf(me.ID, me.AvatarID, me.Profile)))
}

// listClubSuggestions are openvk's "Предложенные записи" of a club, for
// its admins, or with ?mine=true "Предложенные вами записи". Those whose
// author's page is deleted are left out, but counted.
func (a *api) listClubSuggestions(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	page, err := a.posts.Suggestions(r.Context(), token, c.Club.ID, query.Get("mine") == "true", query.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, len(page.Suggestions))
	for i, s := range page.Suggestions {
		ids[i] = s.AuthorID
	}

	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := suggestionsResponse{Suggestions: make([]suggestionJSON, 0, len(page.Suggestions)), NextCursor: page.NextCursor, Total: page.Total}
	for _, s := range page.Suggestions {
		if author, ok := people[s.AuthorID]; ok && !author.Club {
			resp.Suggestions = append(resp.Suggestions, toSuggestionJSON(s, author))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// publishSuggestion is an admin's "Опубликовать запись": the post it
// makes, as the wall shows it.
func (a *api) publishSuggestion(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req publishRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.PublishSuggestion(r.Context(), token, r.PathValue("id"), req.Text, req.Signed)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	posts, err := a.withPeople(r, []model.Post{p})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if len(posts) == 0 {
		// Its author's page was deleted meanwhile: it is not shown.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusCreated, posts[0])
}

// deleteSuggestion is the author's withdrawal or an admin's "Отклонить".
func (a *api) deleteSuggestion(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.posts.DeleteSuggestion(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// suggestionText is what a suggestion says, whole: the list carries its
// beginning. For its author and the admins of the club, as the list is.
func (a *api) suggestionText(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	s, err := a.posts.Suggestion(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, textResponse{Text: s.Text})
}
