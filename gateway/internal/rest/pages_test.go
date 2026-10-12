package rest_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func webDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"index.html":          `INDEX <link rel="stylesheet" href="/static/style.css">`,
		"edit.html":           "EDIT",
		"blacklist.html":      "BLACKLIST",
		"settings.html":       "SETTINGS",
		"wall.html":           "WALL",
		"profile.html":        "PROFILE",
		"post.html":           "POST",
		"notifications.html":  "NOTIFICATIONS",
		"im.html":             "DIALOGS",
		"dialog.html":         "DIALOG",
		"chat.html":           "CHAT",
		"groups.html":         "GROUPS",
		"about.html":          "ABOUT",
		"terms.html":          "TERMS",
		"help.html":           "HELP",
		"club-create.html":    "CLUB_CREATE",
		"event-create.html":   "EVENT_CREATE",
		"club.html":           "CLUB",
		"club-edit.html":      "CLUB_EDIT",
		"club-members.html":   "CLUB_MEMBERS",
		"club-suggested.html": "CLUB_SUGGESTED",
		"notes.html":          "NOTES",
		"fave.html":           "FAVE",
		"note.html":           "NOTE",
		"note-edit.html":      "NOTE_EDIT",
		"static/style.css":    "body{}",
		"static/favicon.png":  "ICON",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestPages(t *testing.T) {
	t.Parallel()

	handler := mustServer(
		rest.Config{RequestTimeout: time.Second, WebDir: webDir(t)},
		rest.Backends{Auth: &fakeAuth{}, Users: &fakeUsers{}, Posts: &fakePosts{}, Media: &fakeMedia{}, Notifications: &fakeNotifications{}, Messages: &fakeMessages{}, Clubs: &fakeClubs{}},
		prometheus.NewRegistry(),
	).Handler()

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}

	tests := []struct {
		path   string
		status int
		body   string
		csp    bool
	}{
		{"/", 200, "INDEX", true},
		{"/edit", 200, "EDIT", true},
		{"/settings", 200, "SETTINGS", true},
		{"/wall/ann", 200, "WALL", true},
		{"/club/cats/wall", 200, "WALL", true},
		{"/settings/blacklist", 200, "BLACKLIST", true},
		{"/ann", 200, "PROFILE", true},
		{"/post/p1", 200, "POST", true},
		{"/notifications", 200, "NOTIFICATIONS", true},
		{"/im", 200, "DIALOGS", true},
		{"/im/bob", 200, "DIALOG", true},
		{"/im/chat/0198c0de-0000-7000-8000-000000000001", 200, "CHAT", true},
		{"/groups", 200, "GROUPS", true},
		{"/about", 200, "ABOUT", true},
		{"/terms", 200, "TERMS", true},
		{"/help", 200, "HELP", true},
		{"/groups/create", 200, "CLUB_CREATE", true},
		{"/events", 200, "GROUPS", true},
		{"/events/create", 200, "EVENT_CREATE", true},
		{"/club/cats", 200, "CLUB", true},
		{"/club/cats/edit", 200, "CLUB_EDIT", true},
		{"/club/cats/members", 200, "CLUB_MEMBERS", true},
		{"/club/cats/suggested", 200, "CLUB_SUGGESTED", true},
		{"/notes", 200, "NOTES", true},
		{"/fave", 200, "FAVE", true},
		{"/fave/own", 200, "FAVE", true},
		{"/notes/create", 200, "NOTE_EDIT", true},
		{"/note/n1", 200, "NOTE", true},
		{"/note/n1/edit", 200, "NOTE_EDIT", true},
		{"/static/style.css", 200, "body{}", false},
		{"/static/", 404, "", false},
		{"/favicon.ico", 200, "ICON", false},
		{"/api/v1/users/ann", 200, `"nickname":"ann"`, false},
		{"/api/v1/nope", 404, `"not_found"`, false},
		{"/healthz", 200, "ok", false},
	}

	for _, tt := range tests {
		// Not in parallel: the requests share the fake backends, which
		// remember what they were asked.
		t.Run(tt.path, func(t *testing.T) {
			w := get(tt.path)
			if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.body) {
				t.Fatalf("%s: %d %q", tt.path, w.Code, w.Body.String())
			}
			if hasCSP := w.Header().Get("Content-Security-Policy") != ""; hasCSP != tt.csp {
				t.Fatalf("%s: CSP present = %v, want %v", tt.path, hasCSP, tt.csp)
			}
		})
	}

	// The site may be cached, the API never: a page is checked with the
	// server each time and names the versions of its assets, which are
	// then kept for good.
	t.Run("pages name the versions of their assets", func(t *testing.T) {
		t.Parallel()

		w := get("/")

		_, rest, found := strings.Cut(w.Body.String(), `href="/static/c/`)
		version, _, _ := strings.Cut(rest, "/style.css")

		if !found || version == "" || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("page: %q, Cache-Control %q", w.Body.String(), w.Header().Get("Cache-Control"))
		}

		asset := get("/static/c/" + version + "/style.css")
		if cc := asset.Header().Get("Cache-Control"); asset.Code != 200 || asset.Body.String() != "body{}" || !strings.Contains(cc, "immutable") {
			t.Fatalf("asset of the version: %d %q, Cache-Control %q", asset.Code, asset.Body.String(), cc)
		}

		// A page of another build gets the file, but not for good: by the
		// version of the file it knew and by that of its whole site.
		for _, path := range []string{"/static/c/000000000000/style.css", "/static/v/000000000000/style.css"} {
			other := get(path)
			if other.Code != 200 || other.Body.String() != "body{}" || other.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("%s: %d, Cache-Control %q", path, other.Code, other.Header().Get("Cache-Control"))
			}
		}

		if plain := get("/static/style.css"); plain.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("asset at its plain path: Cache-Control %q", plain.Header().Get("Cache-Control"))
		}

		if api := get("/api/v1/users/ann"); api.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("API: Cache-Control %q", api.Header().Get("Cache-Control"))
		}

		for _, path := range []string{"/static/c/" + version + "/", "/static/v/" + version + "/", "/static/c/" + version + "/none.css"} {
			if none := get(path); none.Code != http.StatusNotFound {
				t.Fatalf("%s: %d", path, none.Code)
			}
		}
	})

	t.Run("an unchanged page is not sent again", func(t *testing.T) {
		t.Parallel()

		etag := get("/").Header().Get("ETag")
		if etag == "" {
			t.Fatal("a page has no ETag")
		}

		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("If-None-Match", etag)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
		}
	})

	t.Run("without WEB_DIR only the API is served", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do(http.MethodGet, "/", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})
}
