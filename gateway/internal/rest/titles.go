package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Titles are the awards the administrators give pages, which the user
// service keeps with the people.
type Titles interface {
	UserTitles(ctx context.Context, accessToken, nickname string) ([]model.Title, error)
	Grant(ctx context.Context, accessToken, nickname, name, icon string) (model.Title, error)
	Revoke(ctx context.Context, accessToken, id string) error
}

type titleJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`
	GrantedAt time.Time `json:"grantedAt"`
}

func toTitleJSON(t model.Title) titleJSON {
	return titleJSON{ID: t.ID, Name: t.Name, Icon: t.Icon, GrantedAt: t.GrantedAt}
}

// getUserTitles are the titles of a page, public as the page.
func (a *api) getUserTitles(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	titles, err := a.titles.UserTitles(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Titles []titleJSON `json:"titles"`
	}{listJSON(titles, toTitleJSON)})
}

// grantTitle gives a page a title; the administrators'.
func (a *api) grantTitle(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	title, err := a.titles.Grant(r.Context(), token, r.PathValue("nickname"), req.Name, req.Icon)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toTitleJSON(title))
}

// revokeTitle takes a title away; the administrators'.
func (a *api) revokeTitle(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.titles.Revoke(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
