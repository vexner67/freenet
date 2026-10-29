package rest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
)

// pageCSP allows only our own scripts and styles and no inline code, so
// markup that slips into a page cannot run. Pages and API share the origin.
// Trusted Types close the sinks that turn a string into markup or a script
// (innerHTML and the like) but to the two named policies of the site: the
// theme's link (theme.js) and the service worker (push.js).
const pageCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; " +
	"require-trusted-types-for 'script'; trusted-types freenet-theme freenet-sw"

// The website holds nothing of a person, so, unlike the API, it may be
// cached. A page and an asset at its plain path are checked with the
// server every time (a 304 when nothing changed); an asset at the path
// that names its version never changes and is kept for good.
const (
	cacheRevalidate = "no-cache"
	cacheForever    = "public, max-age=31536000, immutable"

	staticPrefix = "/static/"
	// buildPrefix is where the pages of the builds before named the
	// version of the whole site: such a page, still open, gets the files
	// as they are, to be checked again.
	buildPrefix = "/static/v/"
)

// registerPages serves the website from dir: pages at fixed paths and
// assets under /static/. Profiles live at /<nickname>; the user service
// reserves nicknames that would clash with these paths.
func registerPages(mux *http.ServeMux, dir string) {
	site := &assets{dir: filepath.Join(dir, "static")}
	files := noListing(http.FileServer(http.Dir(site.dir)))

	mux.Handle("GET /static/", cached(cacheRevalidate, http.StripPrefix(staticPrefix, files)))
	mux.Handle("GET /static/c/{version}/{path...}", site.content())
	mux.Handle("GET /static/v/{version}/{path...}", cached(cacheRevalidate, earlier(files)))

	// Browsers ask for it whatever the pages say; without it the request
	// would open the page of a person named so.
	mux.Handle("GET /favicon.ico", cached(cacheRevalidate, asset(dir, "favicon.png")))
	mux.Handle("GET /sw.js", site.worker())

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
	mux.Handle("GET /language", site.page(dir, "language.html"))
	mux.Handle("GET /invite", site.page(dir, "invite.html"))
	mux.Handle("GET /groups", site.page(dir, "groups.html"))
	mux.Handle("GET /groups/create", site.page(dir, "club-create.html"))
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
	mux.Handle("GET /gifts", site.page(dir, "gifts.html"))
	mux.Handle("GET /gift/{id}", site.page(dir, "gift.html"))
	mux.Handle("GET /market", site.page(dir, "market.html"))
	mux.Handle("GET /shop", site.page(dir, "stickers.html"))
	mux.Handle("GET /stickers", site.page(dir, "stickers.html"))
	mux.Handle("GET /docs", site.page(dir, "docs.html"))
	mux.Handle("GET /articles", site.page(dir, "articles.html"))
	mux.Handle("GET /article/new", site.page(dir, "article-edit.html"))
	mux.Handle("GET /article/{id}", site.page(dir, "article.html"))
	mux.Handle("GET /article/{id}/edit", site.page(dir, "article-edit.html"))
	mux.Handle("GET /casino", site.page(dir, "casino.html"))
	mux.Handle("GET /mine", site.page(dir, "mine.html"))
	// The guests are shown among the notifications now.
	mux.Handle("GET /guests", http.RedirectHandler("/notifications?box=archive", http.StatusMovedPermanently))
	mux.Handle("GET /top", site.page(dir, "top.html"))
	mux.Handle("GET /pet", site.page(dir, "pet.html"))
	mux.Handle("GET /canvas", site.page(dir, "canvas.html"))
	mux.Handle("GET /dating", site.page(dir, "dating.html"))
	mux.Handle("GET /business", site.page(dir, "business.html"))
	mux.Handle("GET /stocks", site.page(dir, "stocks.html"))
	mux.Handle("GET /votes", site.page(dir, "votes.html"))
	mux.Handle("GET /prime", site.page(dir, "prime.html"))
	mux.Handle("GET /admin", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/vouchers", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/vouchers/new", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/users", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/clubs", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/bans", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/gifts", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/decor", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/casino", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/prime", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/bets", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/mine", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/pets", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/business", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/bank", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/ads", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/ads/new", site.page(dir, "admin.html"))
	mux.Handle("GET /admin/ads/{id}", site.page(dir, "admin.html"))
	mux.Handle("GET /{nickname}", site.page(dir, "profile.html"))
}

// asset serves one file of /static/ at a path of its own.
func asset(dir, name string) http.Handler {
	path := filepath.Join(dir, "static", name)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	})
}

// worker serves the service worker of the site (static/sw.js) at the root:
// a worker acts on the pages under its own path, here all of them. It is
// checked every time, as a page: a worker kept for good would never be
// replaced.
func (a *assets) worker() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, ok := a.current()["sw.js"]
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", cacheRevalidate)
		w.Header().Set("ETag", `"`+file.version+`"`)
		http.ServeContent(w, r, "sw.js", time.Time{}, bytes.NewReader(file.content))
	})
}

// The pages of a phone (web/m/) are those of a computer laid out anew
// for a narrow screen, as m.vk.com was: the same addresses, scripts and
// API. A phone gets one where there is one, the computer's elsewhere; the
// cookie "site" keeps what the person chose at the bottom of a page —
// "full" or "m" — over what the browser says it is.
const (
	phoneDir   = "m"
	siteCookie = "site"
	siteFull   = "full"
	sitePhone  = "m"
)

// phoneAgent is what the browsers of phones say they are. A tablet is
// wide enough for the pages of a computer: an Android one does not say
// "Mobile", an iPad does and is told apart.
var phoneAgent = regexp.MustCompile(`Mobi|iPhone|iPod|Opera Mini`)

// wantsPhone is whether the request asks for the pages of a phone.
func wantsPhone(r *http.Request) bool {
	if c, err := r.Cookie(siteCookie); err == nil {
		switch c.Value {
		case siteFull:
			return false
		case sitePhone:
			return true
		}
	}

	agent := r.UserAgent()

	return phoneAgent.MatchString(agent) && !strings.Contains(agent, "iPad")
}

// page serves a page with the paths of its assets naming their versions:
// a phone's own one when it asks for it and there is one.
func (a *assets) page(dir, name string) http.Handler {
	path := filepath.Join(dir, name)
	phone := filepath.Join(dir, phoneDir, name)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The same address is another page for a phone.
		w.Header().Add("Vary", "Cookie, User-Agent")

		html, err := readPage(path, phone, wantsPhone(r))
		if err != nil {
			http.NotFound(w, r)
			return
		}

		site := a.current()
		html = site.versioned(site.preloading(html))

		w.Header().Set("Content-Security-Policy", pageCSP)
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", cacheRevalidate)
		w.Header().Set("ETag", `"`+digestOf(html)+`"`)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(html))
	})
}

// readPage reads the page of a phone when asked and there is one, the
// page of a computer otherwise.
func readPage(path, phone string, isPhone bool) ([]byte, error) {
	if isPhone {
		html, err := os.ReadFile(phone) //nolint:gosec // a fixed file of the website
		if err == nil {
			return html, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, errs.Wrap("os.ReadFile", err)
		}
	}

	html, err := os.ReadFile(path) //nolint:gosec // a fixed file of the website
	if err != nil {
		return nil, errs.Wrap("os.ReadFile", err)
	}

	return html, nil
}

// moduleScript is how a page loads its script.
var moduleScript = regexp.MustCompile(`<script type="module" src="/static/([\w./-]+)"></script>`)

// preloading names, before the script of the page, every module the
// script imports however deep: the browser then asks for them all at
// once instead of learning of each from the one before it.
func (s siteFiles) preloading(html []byte) []byte {
	return moduleScript.ReplaceAllFunc(html, func(found []byte) []byte {
		var links bytes.Buffer

		for _, name := range s.preloaded(string(moduleScript.FindSubmatch(found)[1])) {
			links.WriteString(`<link rel="modulepreload" href="` + staticPrefix + name + `">` + "\n  ")
		}

		return append(links.Bytes(), found...)
	})
}

// earlier serves a file to a page of a build before this one.
func earlier(files http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix(buildPrefix+r.PathValue("version")+"/", files).ServeHTTP(w, r)
	})
}

func digestOf(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:versionLength/2])
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
