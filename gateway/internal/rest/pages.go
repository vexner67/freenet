package rest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pageCSP allows only our own scripts and styles and no inline code, so
// markup that slips into a page cannot run. Pages and API share the origin.
const pageCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// The website holds nothing of a person, so, unlike the API, it may be
// cached. A page and an asset at its plain path are checked with the
// server every time (a 304 when nothing changed); an asset at the path
// that names the version of the site never changes and is kept for good.
const (
	cacheRevalidate = "no-cache"
	cacheForever    = "public, max-age=31536000, immutable"

	staticPrefix    = "/static/"
	versionedPrefix = "/static/v/"

	// assetsCheckInterval is how long a version of the site is believed
	// before its files are looked at again.
	assetsCheckInterval = time.Second
)

// assets knows the version of the files under /static/: a digest of their
// names, sizes and times, so a new build of the site (and a file saved
// while developing) is a new version. Pages name it in the paths of
// their assets, which lets a browser keep those for good and still get
// the new ones with the next page.
type assets struct {
	dir string

	mu      sync.Mutex
	version string
	checked time.Time
}

func (a *assets) current() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.version != "" && time.Since(a.checked) < assetsCheckInterval {
		return a.version
	}

	digest := sha256.New()

	// A file that cannot be read leaves the version as the others make
	// it: the request for it answers for itself.
	_ = filepath.WalkDir(a.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // see above
		}

		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // see above
		}

		digest.Write([]byte(path + " " + strconv.FormatInt(info.Size(), 10) + " " + strconv.FormatInt(info.ModTime().UnixNano(), 10) + "\n"))

		return nil
	})

	a.version = hex.EncodeToString(digest.Sum(nil))[:12]
	a.checked = time.Now()

	return a.version
}

// registerPages serves the website from dir: pages at fixed paths and
// assets under /static/. Profiles live at /<nickname>; the user service
// reserves nicknames that would clash with these paths.
func registerPages(mux *http.ServeMux, dir string) {
	site := &assets{dir: filepath.Join(dir, "static")}
	files := noListing(http.FileServer(http.Dir(site.dir)))

	mux.Handle("GET /static/", cached(cacheRevalidate, http.StripPrefix(staticPrefix, files)))
	mux.Handle("GET /static/v/{version}/{path...}", site.versioned(files))

	// Browsers ask for it whatever the pages say; without it the request
	// would open the page of a person named so.
	mux.Handle("GET /favicon.ico", cached(cacheRevalidate, asset(dir, "favicon.png")))

	mux.Handle("GET /{$}", site.page(dir, "index.html"))
	mux.Handle("GET /edit", site.page(dir, "edit.html"))
	mux.Handle("GET /settings", site.page(dir, "settings.html"))
	mux.Handle("GET /settings/blacklist", site.page(dir, "blacklist.html"))
	mux.Handle("GET /feed", site.page(dir, "feed.html"))
	mux.Handle("GET /feed/all", site.page(dir, "feed.html"))
	mux.Handle("GET /follows", site.page(dir, "follows.html"))
	mux.Handle("GET /search", site.page(dir, "search.html"))
	mux.Handle("GET /post/{id}", site.page(dir, "post.html"))
	mux.Handle("GET /notifications", site.page(dir, "notifications.html"))
	mux.Handle("GET /im", site.page(dir, "im.html"))
	mux.Handle("GET /im/{nickname}", site.page(dir, "dialog.html"))
	mux.Handle("GET /im/chat/{id}", site.page(dir, "chat.html"))
	mux.Handle("GET /about", site.page(dir, "about.html"))
	mux.Handle("GET /terms", site.page(dir, "terms.html"))
	mux.Handle("GET /help", site.page(dir, "help.html"))
	mux.Handle("GET /groups", site.page(dir, "groups.html"))
	mux.Handle("GET /groups/create", site.page(dir, "club-create.html"))
	// "Мои Встречи" is the page of "Мои Группы" listing events.
	mux.Handle("GET /events", site.page(dir, "groups.html"))
	mux.Handle("GET /events/create", site.page(dir, "event-create.html"))
	mux.Handle("GET /club/{ref}", site.page(dir, "club.html"))
	mux.Handle("GET /club/{ref}/edit", site.page(dir, "club-edit.html"))
	mux.Handle("GET /club/{ref}/members", site.page(dir, "club-members.html"))
	mux.Handle("GET /club/{ref}/backdrop", site.page(dir, "club-backdrop.html"))
	mux.Handle("GET /club/{ref}/suggested", site.page(dir, "club-suggested.html"))
	mux.Handle("GET /club/{ref}/board", site.page(dir, "board.html"))
	mux.Handle("GET /club/{ref}/board/create", site.page(dir, "topic-create.html"))
	mux.Handle("GET /topic/{id}", site.page(dir, "topic.html"))
	mux.Handle("GET /topic/{id}/edit", site.page(dir, "topic-edit.html"))
	mux.Handle("GET /albums", site.page(dir, "albums.html"))
	mux.Handle("GET /albums/create", site.page(dir, "album-create.html"))
	mux.Handle("GET /club/{ref}/albums", site.page(dir, "albums.html"))
	mux.Handle("GET /wall/{nickname}", site.page(dir, "wall.html"))
	mux.Handle("GET /club/{ref}/wall", site.page(dir, "wall.html"))
	mux.Handle("GET /album/{id}", site.page(dir, "album.html"))
	mux.Handle("GET /album/{id}/edit", site.page(dir, "album-edit.html"))
	mux.Handle("GET /album/{id}/upload", site.page(dir, "album-upload.html"))
	mux.Handle("GET /photo/{id}", site.page(dir, "photo.html"))
	mux.Handle("GET /fave", site.page(dir, "fave.html"))
	mux.Handle("GET /fave/own", site.page(dir, "fave.html"))
	mux.Handle("GET /notes", site.page(dir, "notes.html"))
	mux.Handle("GET /notes/create", site.page(dir, "note-edit.html"))
	mux.Handle("GET /note/{id}", site.page(dir, "note.html"))
	mux.Handle("GET /note/{id}/edit", site.page(dir, "note-edit.html"))
	mux.Handle("GET /{nickname}", site.page(dir, "profile.html"))
}

// asset serves one file of /static/ at a path of its own.
func asset(dir, name string) http.Handler {
	path := filepath.Join(dir, "static", name)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	})
}

// page serves a page with the paths of its assets naming the version of
// the site.
func (a *assets) page(dir, name string) http.Handler {
	path := filepath.Join(dir, name)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html, err := os.ReadFile(path) //nolint:gosec // a fixed file of the website
		if err != nil {
			http.NotFound(w, r)
			return
		}

		version := a.current()
		html = bytes.ReplaceAll(html, []byte(`"`+staticPrefix), []byte(`"`+versionedPrefix+version+"/"))

		w.Header().Set("Content-Security-Policy", pageCSP)
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", cacheRevalidate)
		w.Header().Set("ETag", `"`+version+"-"+digestOf(html)+`"`)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(html))
	})
}

// versioned serves an asset at the path naming a version of the site:
// for good when that is the version served, and to be checked again when
// it is not — a page of the build before asked a server of the new one.
func (a *assets) versioned(files http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy := cacheRevalidate
		if r.PathValue("version") == a.current() {
			policy = cacheForever
		}

		w.Header().Set("Cache-Control", policy)
		http.StripPrefix(versionedPrefix+r.PathValue("version")+"/", files).ServeHTTP(w, r)
	})
}

func digestOf(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:6])
}

// cached serves next under a caching policy instead of the API's "never".
func cached(policy string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", policy)
		next.ServeHTTP(w, r)
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
