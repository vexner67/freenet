package rest

import (
	"net/http"
	"path/filepath"
	"strings"
)

// pageCSP allows only our own scripts and styles and no inline code, so
// markup that slips into a page cannot run. Pages and API share the origin.
const pageCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// registerPages serves the website from dir: pages at fixed paths and
// assets under /static/. Profiles live at /<nickname>; the user service
// reserves nicknames that would clash with these paths.
func registerPages(mux *http.ServeMux, dir string) {
	mux.Handle("GET /static/", http.StripPrefix("/static/", noListing(http.FileServer(http.Dir(filepath.Join(dir, "static"))))))

	// Browsers ask for it whatever the pages say; without it the request
	// would open the page of a person named so.
	mux.Handle("GET /favicon.ico", asset(dir, "favicon.png"))

	mux.Handle("GET /{$}", page(dir, "index.html"))
	mux.Handle("GET /edit", page(dir, "edit.html"))
	mux.Handle("GET /settings", page(dir, "settings.html"))
	mux.Handle("GET /settings/blacklist", page(dir, "blacklist.html"))
	mux.Handle("GET /feed", page(dir, "feed.html"))
	mux.Handle("GET /feed/all", page(dir, "feed.html"))
	mux.Handle("GET /follows", page(dir, "follows.html"))
	mux.Handle("GET /search", page(dir, "search.html"))
	mux.Handle("GET /post/{id}", page(dir, "post.html"))
	mux.Handle("GET /notifications", page(dir, "notifications.html"))
	mux.Handle("GET /im", page(dir, "im.html"))
	mux.Handle("GET /im/{nickname}", page(dir, "dialog.html"))
	mux.Handle("GET /about", page(dir, "about.html"))
	mux.Handle("GET /terms", page(dir, "terms.html"))
	mux.Handle("GET /help", page(dir, "help.html"))
	mux.Handle("GET /groups", page(dir, "groups.html"))
	mux.Handle("GET /groups/create", page(dir, "club-create.html"))
	// "Мои Встречи" is the page of "Мои Группы" listing events.
	mux.Handle("GET /events", page(dir, "groups.html"))
	mux.Handle("GET /events/create", page(dir, "event-create.html"))
	mux.Handle("GET /club/{ref}", page(dir, "club.html"))
	mux.Handle("GET /club/{ref}/edit", page(dir, "club-edit.html"))
	mux.Handle("GET /club/{ref}/members", page(dir, "club-members.html"))
	mux.Handle("GET /club/{ref}/backdrop", page(dir, "club-backdrop.html"))
	mux.Handle("GET /club/{ref}/suggested", page(dir, "club-suggested.html"))
	mux.Handle("GET /club/{ref}/board", page(dir, "board.html"))
	mux.Handle("GET /club/{ref}/board/create", page(dir, "topic-create.html"))
	mux.Handle("GET /topic/{id}", page(dir, "topic.html"))
	mux.Handle("GET /topic/{id}/edit", page(dir, "topic-edit.html"))
	mux.Handle("GET /albums", page(dir, "albums.html"))
	mux.Handle("GET /albums/create", page(dir, "album-create.html"))
	mux.Handle("GET /club/{ref}/albums", page(dir, "albums.html"))
	mux.Handle("GET /wall/{nickname}", page(dir, "wall.html"))
	mux.Handle("GET /club/{ref}/wall", page(dir, "wall.html"))
	mux.Handle("GET /album/{id}", page(dir, "album.html"))
	mux.Handle("GET /album/{id}/edit", page(dir, "album-edit.html"))
	mux.Handle("GET /album/{id}/upload", page(dir, "album-upload.html"))
	mux.Handle("GET /photo/{id}", page(dir, "photo.html"))
	mux.Handle("GET /notes", page(dir, "notes.html"))
	mux.Handle("GET /notes/create", page(dir, "note-edit.html"))
	mux.Handle("GET /note/{id}", page(dir, "note.html"))
	mux.Handle("GET /note/{id}/edit", page(dir, "note-edit.html"))
	mux.Handle("GET /{nickname}", page(dir, "profile.html"))
}

// asset serves one file of /static/ at a path of its own.
func asset(dir, name string) http.Handler {
	path := filepath.Join(dir, "static", name)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	})
}

func page(dir, name string) http.Handler {
	path := filepath.Join(dir, name)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", pageCSP)
		w.Header().Set("Referrer-Policy", "same-origin")
		http.ServeFile(w, r, path)
	})
}

func noListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
