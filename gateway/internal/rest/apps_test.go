package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeApps knows g1, Bob's (u2) snake.html, and g2 of a deleted page (u3).
type fakeApps struct {
	token, section, cursor, changed string
}

func (f *fakeApps) app(id string) (model.App, bool) {
	switch id {
	case "g1":
		return model.App{Document: model.Document{ID: "g1", OwnerID: "u2", Name: "snake.html", Ext: "html", CreatedAt: documentTime}, Users: 3, Installed: f.token != ""}, true
	case "g2":
		return model.App{Document: model.Document{ID: "g2", OwnerID: "u3", Name: "old.html", Ext: "html", CreatedAt: documentTime}}, true
	}
	return model.App{}, false
}

func (f *fakeApps) Get(_ context.Context, token, id string) (model.App, error) {
	f.token = token
	app, ok := f.app(id)
	if !ok {
		return model.App{}, failure.New(failure.NotFound, "game not found")
	}
	return app, nil
}

func (f *fakeApps) List(_ context.Context, token, section, cursor string, _ int) (model.AppPage, error) {
	f.token, f.section, f.cursor = token, section, cursor
	g1, _ := f.app("g1")
	g2, _ := f.app("g2")
	return model.AppPage{Apps: []model.App{g1, g2}, NextCursor: "g2"}, nil
}

func (f *fakeApps) Install(_ context.Context, token, id string) error {
	f.token, f.changed = token, "+"+id
	return nil
}

func (f *fakeApps) Uninstall(_ context.Context, token, id string) error {
	f.token, f.changed = token, "-"+id
	return nil
}

type appBody struct {
	ID, Name  string
	Document  struct{ URL string }
	Developer *struct{ ID, Name string }
	Users     int
	Installed bool
}

func TestApps(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/apps?section=installed&cursor=c1", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Apps       []appBody
		NextCursor string
	}](t, w)
	if w.Code != http.StatusOK || len(got.Apps) != 2 || got.NextCursor != "g2" || h.apps.section != "installed" || h.apps.cursor != "c1" {
		t.Fatalf("list: %d %+v", w.Code, got)
	}
	if a := got.Apps[0]; a.Name != "snake" || a.Document.URL != "/media/documents/g1" || a.Developer == nil || a.Developer.Name != "Bob" || a.Users != 3 || !a.Installed {
		t.Fatalf("app = %+v", a)
	}
	// The developer of a deleted page is not shown.
	if got.Apps[1].Developer != nil {
		t.Fatalf("developer of a deleted page = %+v", got.Apps[1].Developer)
	}

	if w = h.do("GET", "/api/v1/apps", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("list as a guest: %d", w.Code)
	}
	if h.do("GET", "/api/v1/apps", "", "Authorization", "Bearer tok"); h.apps.section != model.AppsAll {
		t.Errorf("the default section = %q", h.apps.section)
	}

	// A guest opens a shared app.
	w = h.do("GET", "/api/v1/apps/g1", "")
	if a := decode[appBody](t, w); w.Code != http.StatusOK || a.ID != "g1" || a.Installed {
		t.Fatalf("get as a guest: %d %+v", w.Code, a)
	}
	if w = h.do("GET", "/api/v1/apps/nope", ""); w.Code != http.StatusNotFound {
		t.Errorf("get of nothing: %d", w.Code)
	}

	if w = h.do("POST", "/api/v1/apps/g1/install", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.apps.changed != "+g1" {
		t.Fatalf("install: %d %s", w.Code, h.apps.changed)
	}
	if w = h.do("DELETE", "/api/v1/apps/g1/install", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.apps.changed != "-g1" {
		t.Fatalf("uninstall: %d %s", w.Code, h.apps.changed)
	}
	if w = h.do("POST", "/api/v1/apps/g1/install", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("install as a guest: %d", w.Code)
	}
}
