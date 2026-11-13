package rest

import (
	"context"
	"net/http"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// appJSON is an HTML widget as "Приложения" show it: its name without
// ".html", the document it runs from, its developer, how many added it
// and whether the caller did.
type appJSON struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Document  documentJSON           `json:"document"`
	Developer *publicProfileResponse `json:"developer"`
	Users     int                    `json:"users"`
	Installed bool                   `json:"installed"`
}

type appPageResponse struct {
	Apps       []appJSON `json:"apps"`
	NextCursor string    `json:"nextCursor"`
}

// appName is the name of a widget without its format.
func appName(d model.Document) string {
	if name := strings.TrimSpace(strings.TrimSuffix(d.Name, "."+d.Ext)); name != "" {
		return name
	}

	return d.Name
}

// appsJSON shows apps with their developers; one of a deleted page has
// none.
func (a *api) appsJSON(r *http.Request, apps []model.App) ([]appJSON, error) {
	resp := make([]appJSON, 0, len(apps))
	if len(apps) == 0 {
		return resp, nil
	}

	owners := make([]string, 0, len(apps))
	for _, app := range apps {
		owners = append(owners, app.Document.OwnerID)
	}

	profiles, err := a.users.GetProfiles(r.Context(), uniqueIDs(owners))
	if err != nil {
		return nil, err
	}

	byID := make(map[string]model.PublicProfile, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = p
	}

	for _, app := range apps {
		item := appJSON{
			ID: app.Document.ID, Name: appName(app.Document), Document: toDocumentJSON(app.Document),
			Users: app.Users, Installed: app.Installed,
		}
		if p, ok := byID[app.Document.OwnerID]; ok && !p.Deactivated {
			dev := toPublicProfileResponse(p)
			item.Developer = &dev
		}
		resp = append(resp, item)
	}

	return resp, nil
}

// listApps is a tab of "Приложения": ?section=all (by default), installed
// or own.
func (a *api) listApps(w http.ResponseWriter, r *http.Request) {
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

	section := r.URL.Query().Get("section")
	if section == "" {
		section = model.AppsAll
	}

	page, err := a.apps.List(r.Context(), token, section, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	apps, err := a.appsJSON(r, page.Apps)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, appPageResponse{Apps: apps, NextCursor: page.NextCursor})
}

// getApp is the page of an app, open to guests too: its link is shared.
func (a *api) getApp(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	app, err := a.apps.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	apps, err := a.appsJSON(r, []model.App{app})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, apps[0])
}

// installApp is "Добавить к себе".
func (a *api) installApp(w http.ResponseWriter, r *http.Request) {
	a.changeApp(w, r, a.apps.Install)
}

func (a *api) uninstallApp(w http.ResponseWriter, r *http.Request) {
	a.changeApp(w, r, a.apps.Uninstall)
}

func (a *api) changeApp(w http.ResponseWriter, r *http.Request, change func(ctx context.Context, token, id string) error) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = change(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
