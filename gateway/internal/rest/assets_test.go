package rest_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// A small site: a page that names every file, modules importing each
// other (posts.js and share.js in a circle) and scripts naming files by
// their whole paths.
func siteFiles() map[string]string {
	return map[string]string{
		"index.html": `<link href="/static/style.css"> <script src="/static/theme.js"></script> <script type="module" src="/static/page.js"></script>` +
			` "/static/layout.js" "/static/posts.js" "/static/share.js" "/static/other.js" "/static/club.svg" "/static/themes/dark.css" "/static/none.png"`,
		"static/style.css":       "body{}",
		"static/theme.js":        "const dark = '/static/themes/dark.css';",
		"static/themes/dark.css": "body{color:#fff}",
		"static/page.js":         "import { club } from './layout.js';\nimport './posts.js';\nconst none = '/static/none.png';",
		"static/layout.js":       "export const club = '/static/club.svg';",
		"static/posts.js":        `import "./share.js";` + "\nexport { club } from './layout.js';",
		"static/share.js":        "import './posts.js';",
		"static/other.js":        "export const text = 'import it from \"./nowhere.js\"';",
		"static/club.svg":        "<svg/>",
	}
}

// served is a gateway serving files as its website.
func served(t *testing.T, files map[string]string) func(path string) *httptest.ResponseRecorder {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	handler := mustServer(
		rest.Config{RequestTimeout: time.Second, WebDir: dir},
		rest.Backends{Auth: &fakeAuth{}, Users: &fakeUsers{}, Posts: &fakePosts{}, Media: &fakeMedia{}, Notifications: &fakeNotifications{}, Messages: &fakeMessages{}, Clubs: &fakeClubs{}},
		prometheus.NewRegistry(),
	).Handler()

	return func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

		return w
	}
}

var versionedPath = regexp.MustCompile(`/static/c/([0-9a-f]{12})/([\w./-]+)`)

// versionsIn are the versions text gives to the files it names.
func versionsIn(text string) map[string]string {
	versions := map[string]string{}
	for _, m := range versionedPath.FindAllStringSubmatch(text, -1) {
		versions[m[2]] = m[1]
	}

	return versions
}

func TestAssets(t *testing.T) {
	t.Parallel()

	get := served(t, siteFiles())

	page := get("/").Body.String()
	versions := versionsIn(page)

	if len(versions) != 9 || !strings.Contains(page, `"/static/none.png"`) {
		t.Fatalf("the page names %v: every file by its version, and no file as it was: %s", versions, page)
	}

	file := func(name string) string {
		t.Helper()

		w := get("/static/c/" + versions[name] + "/" + name)
		if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
			t.Fatalf("%s: %d, Cache-Control %q", name, w.Code, w.Header().Get("Cache-Control"))
		}

		return w.Body.String()
	}

	// A file names the others by the versions the page names them by: a
	// module has one address, whoever imports it.
	for name, others := range map[string][]string{
		"page.js":   {"layout.js", "posts.js"},
		"layout.js": {"club.svg"},
		"posts.js":  {"share.js", "layout.js"},
		"share.js":  {"posts.js"},
		"theme.js":  {"themes/dark.css"},
	} {
		named := versionsIn(file(name))
		if len(named) != len(others) {
			t.Fatalf("%s names %v, want %v", name, named, others)
		}

		for _, other := range others {
			if named[other] != versions[other] {
				t.Fatalf("%s names %s by %q, the page by %q", name, other, named[other], versions[other])
			}
		}
	}

	if got := file("page.js"); !strings.Contains(got, "import { club } from '/static/c/") || !strings.Contains(got, "'/static/none.png'") {
		t.Fatalf("page.js: %s", got)
	}

	// What is neither an import of a file nor a path of one stays.
	if got := file("other.js"); got != siteFiles()["static/other.js"] {
		t.Fatalf("other.js: %s", got)
	}
	if got := file("club.svg"); got != "<svg/>" {
		t.Fatalf("club.svg: %s", got)
	}

	if versions["posts.js"] != versions["share.js"] {
		t.Fatalf("modules of one circle: %q and %q", versions["posts.js"], versions["share.js"])
	}

	if w := get("/static/c/" + versions["page.js"] + "/page.js"); w.Header().Get("ETag") == "" || w.Header().Get("Content-Type") == "" {
		t.Fatalf("headers: %v", w.Header())
	}
}

// A changed file is fetched again with the files that name it, and no
// other.
func TestAssets_Changed(t *testing.T) {
	t.Parallel()

	versions := func(change func(files map[string]string)) map[string]string {
		files := siteFiles()
		change(files)

		return versionsIn(served(t, files)("/").Body.String())
	}

	before := versions(func(map[string]string) {})

	tests := []struct {
		name    string
		change  func(files map[string]string)
		changed []string
	}{
		{"nothing", func(map[string]string) {}, nil},
		{"a page", func(f map[string]string) { f["edit.html"] = "EDIT" }, nil},
		{"a stylesheet", func(f map[string]string) { f["static/style.css"] = "body{margin:0}" }, []string{"style.css"}},
		{"a script nobody imports", func(f map[string]string) { f["static/page.js"] += "\n" }, []string{"page.js"}},
		{"a theme", func(f map[string]string) { f["static/themes/dark.css"] = "body{color:#eee}" }, []string{"themes/dark.css", "theme.js"}},
		{"a module of a circle", func(f map[string]string) { f["static/share.js"] += "\n" }, []string{"share.js", "posts.js", "page.js"}},
		{"a picture", func(f map[string]string) { f["static/club.svg"] = "<svg></svg>" }, []string{"club.svg", "layout.js", "posts.js", "share.js", "page.js"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var changed []string

			for name, version := range versions(tt.change) {
				if version != before[name] {
					changed = append(changed, name)
				}
			}

			slices.Sort(changed)
			slices.Sort(tt.changed)

			if !slices.Equal(changed, tt.changed) {
				t.Fatalf("changed %v, want %v", changed, tt.changed)
			}
		})
	}
}

// The website itself: every file a page or a script names is named by its
// version, and no module is left imported beside the script — it would be
// loaded twice, once by each address.
func TestAssets_Website(t *testing.T) {
	t.Parallel()

	web := filepath.Join("..", "..", "..", "web")

	handler := mustServer(
		rest.Config{RequestTimeout: time.Second, WebDir: web},
		rest.Backends{Auth: &fakeAuth{}, Users: &fakeUsers{}, Posts: &fakePosts{}, Media: &fakeMedia{}, Notifications: &fakeNotifications{}, Messages: &fakeMessages{}, Clubs: &fakeClubs{}},
		prometheus.NewRegistry(),
	).Handler()

	get := func(path string) string {
		t.Helper()

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, w.Code)
		}

		return w.Body.String()
	}

	var (
		plain    = regexp.MustCompile("['\"`]/static/[\\w./-]+['\"`]")
		beside   = regexp.MustCompile(`(?:from|import)\s*['"]\./`)
		versions = map[string]string{}
		seen     = map[string]bool{}
		queue    = []string{"/", "/feed", "/im", "/groups", "/albums", "/settings", "/ann"}
	)

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		text := get(path)
		for _, named := range plain.FindAllString(text, -1) {
			if !strings.HasPrefix(named[1:], "/static/c/") {
				t.Fatalf("%s names %s without a version", path, named)
			}
		}
		if left := beside.FindString(text); left != "" && strings.HasSuffix(path, ".js") {
			t.Fatalf("%s still imports a module beside it: %s", path, left)
		}

		for name, version := range versionsIn(text) {
			if known, ok := versions[name]; ok && known != version {
				t.Fatalf("%s names %s by %q, another file by %q", path, name, version, known)
			}

			versions[name] = version

			if !seen[name] && (strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css")) {
				seen[name] = true
				queue = append(queue, "/static/c/"+version+"/"+name)
			}
		}
	}

	for _, name := range []string{"layout.js", "api.js", "posts.js", "share.js", "style.css", "theme.js", "themes/dark.css", "club.svg", "avatar.svg", "chat.svg", "favicon.svg"} {
		if versions[name] == "" {
			t.Errorf("%s is not named by its version anywhere", name)
		}
	}
}
