package rest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A file of /static/ is served for good at the path that names its own
// version, a digest of what the file says: a new build of the site makes
// browsers fetch only the files that changed.
//
// A file names another in two ways, and the server puts the version of
// the other into both: a module imports a module beside it
// (from './layout.js'), and a page, a script or a stylesheet names a file
// by its whole path, as one string ("/static/club.svg"). So the version
// of a file covers the files it names as well: a script that imports a
// changed one changes with it. Modules may import each other in a circle;
// those of one circle share a version made of them all.
var (
	importPattern = regexp.MustCompile(`((?:from|import)\s*)(['"])\./([\w./-]+)(['"])`)
	staticPattern = regexp.MustCompile("(['\"`])" + staticPrefix + "([\\w./-]+)(['\"`])")
)

const (
	contentPrefix = "/static/c/"

	// assetsCheckInterval is how long the files are believed unchanged
	// before they are looked at again.
	assetsCheckInterval = time.Second

	versionLength = 12
)

type staticFile struct {
	version string
	// content is the file as served at its version: with the versions of
	// the files it names.
	content []byte
	// imports are the modules a module imports, by their paths in /static/.
	imports []string
}

// siteFiles are the files of /static/ as last read, never changed afterwards.
type siteFiles map[string]staticFile

// versioned puts the versions of the files into the whole paths text
// names; a path of no file stays as it is.
func (s siteFiles) versioned(text []byte) []byte {
	return staticPattern.ReplaceAllFunc(text, func(found []byte) []byte {
		m := staticPattern.FindSubmatch(found)

		a, ok := s[string(m[2])]
		if !ok || !bytes.Equal(m[1], m[3]) {
			return found
		}

		return []byte(string(m[1]) + contentPrefix + a.version + "/" + string(m[2]) + string(m[3]))
	})
}

// assets reads the files under dir again once they change: a file saved
// while developing is served at once.
type assets struct {
	dir string

	mu      sync.Mutex
	site    siteFiles
	state   string
	checked time.Time
}

func (a *assets) current() siteFiles {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.site != nil && time.Since(a.checked) < assetsCheckInterval {
		return a.site
	}

	a.checked = time.Now()

	if state := a.stat(); a.site == nil || state != a.state {
		a.site, a.state = newSite(a.read()), state
	}

	return a.site
}

// stat tells the files apart by their names, sizes and times, without
// reading them.
func (a *assets) stat() string {
	digest := sha256.New()

	a.walk(func(name string, d fs.DirEntry) {
		if info, err := d.Info(); err == nil {
			digest.Write([]byte(name + " " + strconv.FormatInt(info.Size(), 10) + " " + strconv.FormatInt(info.ModTime().UnixNano(), 10) + "\n"))
		}
	})

	return hex.EncodeToString(digest.Sum(nil))
}

func (a *assets) read() map[string][]byte {
	files := map[string][]byte{}

	a.walk(func(name string, _ fs.DirEntry) {
		// A file that cannot be read is not of the site: the request for
		// it answers for itself.
		if content, err := os.ReadFile(filepath.Join(a.dir, filepath.FromSlash(name))); err == nil { //nolint:gosec // a file of the website
			files[name] = content
		}
	})

	return files
}

// walk visits the files under dir by their paths in /static/.
func (a *assets) walk(visit func(name string, d fs.DirEntry)) {
	_ = filepath.WalkDir(a.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // what cannot be read is left out
		}

		if name, err := filepath.Rel(a.dir, p); err == nil {
			visit(filepath.ToSlash(name), d)
		}

		return nil
	})
}

// newSite gives every file its version, the files a file names first.
func newSite(files map[string][]byte) siteFiles {
	names := make(map[string][]string, len(files))
	for name, content := range files {
		names[name] = named(name, content, files)
	}

	s := make(siteFiles, len(files))

	for _, circle := range circles(names) {
		digest := sha256.New()

		var others []string

		for _, name := range circle {
			digest.Write([]byte(name + "\n"))
			digest.Write(files[name])

			for _, other := range names[name] {
				if !slices.Contains(circle, other) {
					others = append(others, other)
				}
			}
		}

		slices.Sort(others)

		for _, other := range slices.Compact(others) {
			digest.Write([]byte("\n" + other + " " + s[other].version))
		}

		version := hex.EncodeToString(digest.Sum(nil))[:versionLength]
		for _, name := range circle {
			s[name] = staticFile{version: version}
		}

		for _, name := range circle {
			content := files[name]
			if isText(name) {
				content = s.versioned(s.imported(name, content))
			}

			s[name] = staticFile{version: version, content: content, imports: imports(name, files[name], files)}
		}
	}

	return s
}

func isText(name string) bool {
	return strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css")
}

// named are the files of the site the file names, each once, in order.
func named(name string, content []byte, files map[string][]byte) []string {
	if !isText(name) {
		return nil
	}

	var found []string

	for _, m := range importPattern.FindAllSubmatch(content, -1) {
		found = append(found, path.Join(path.Dir(name), string(m[3])))
	}

	for _, m := range staticPattern.FindAllSubmatch(content, -1) {
		found = append(found, string(m[2]))
	}

	found = slices.DeleteFunc(found, func(other string) bool {
		_, ok := files[other]

		return !ok
	})

	slices.Sort(found)

	return slices.Compact(found)
}

// imports are the modules of the site the script imports, each once.
func imports(name string, content []byte, files map[string][]byte) []string {
	if !strings.HasSuffix(name, ".js") {
		return nil
	}

	var found []string

	for _, m := range importPattern.FindAllSubmatch(content, -1) {
		if other := path.Join(path.Dir(name), string(m[3])); files[other] != nil {
			found = append(found, other)
		}
	}

	slices.Sort(found)

	return slices.Compact(found)
}

// preloaded are the modules the module name imports, and those they
// import, all the way down: what a page loading it waits for.
func (s siteFiles) preloaded(name string) []string {
	seen := map[string]bool{name: true}
	queue := []string{name}

	var out []string

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		for _, other := range s[next].imports {
			if !seen[other] {
				seen[other] = true
				out = append(out, other)
				queue = append(queue, other)
			}
		}
	}

	return out
}

// imported puts the versions into the imports of the modules beside the
// script: a module has one address, whoever imports it.
func (s siteFiles) imported(name string, content []byte) []byte {
	if !strings.HasSuffix(name, ".js") {
		return content
	}

	return importPattern.ReplaceAllFunc(content, func(found []byte) []byte {
		m := importPattern.FindSubmatch(found)
		other := path.Join(path.Dir(name), string(m[3]))

		a, ok := s[other]
		if !ok {
			return found
		}

		return []byte(string(m[1]) + string(m[2]) + contentPrefix + a.version + "/" + other + string(m[4]))
	})
}

// circles groups the files that name each other in a circle (most are
// alone in theirs) and puts the groups so that a file comes after the
// files it names: Tarjan's strongly connected components. The order does
// not depend on the order of the map.
func circles(names map[string][]string) [][]string {
	var (
		out     [][]string
		stack   []string
		counter int
		index   = map[string]int{}
		low     = map[string]int{}
		onStack = map[string]bool{}
		visit   func(name string)
	)

	visit = func(name string) {
		counter++
		index[name], low[name] = counter, counter
		stack, onStack[name] = append(stack, name), true

		for _, other := range names[name] {
			if index[other] == 0 {
				visit(other)
				low[name] = min(low[name], low[other])
			} else if onStack[other] {
				low[name] = min(low[name], index[other])
			}
		}

		if low[name] != index[name] {
			return
		}

		at := slices.Index(stack, name)
		circle := slices.Clone(stack[at:])
		stack = stack[:at]

		for _, member := range circle {
			onStack[member] = false
		}

		slices.Sort(circle)
		out = append(out, circle)
	}

	all := make([]string, 0, len(names))
	for name := range names {
		all = append(all, name)
	}

	slices.Sort(all)

	for _, name := range all {
		if index[name] == 0 {
			visit(name)
		}
	}

	return out
}

// content serves a file at the path naming a version: for good when that
// is the version of the file, and to be checked again when it is not — a
// page of the build before asked a server of the new one.
func (a *assets) content() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("path")

		file, ok := a.current()[name]
		if !ok {
			http.NotFound(w, r)
			return
		}

		policy := cacheRevalidate
		if r.PathValue("version") == file.version {
			policy = cacheForever
		}

		w.Header().Set("Cache-Control", policy)
		w.Header().Set("ETag", `"`+file.version+`"`)
		http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(file.content))
	})
}
