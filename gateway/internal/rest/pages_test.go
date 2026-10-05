package rest_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

func webDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"index.html":       "INDEX",
		"edit.html":        "EDIT",
		"profile.html":     "PROFILE",
		"static/style.css": "body{}",
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

	handler := rest.NewServer(
		rest.Config{RequestTimeout: time.Second, WebDir: webDir(t)},
		&fakeAuth{}, &fakeUsers{}, &fakePosts{}, &fakeMedia{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
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
		{"/ann", 200, "PROFILE", true},
		{"/static/style.css", 200, "body{}", false},
		{"/static/", 404, "", false},
		{"/api/v1/users/ann", 200, `"nickname":"ann"`, false},
		{"/api/v1/nope", 404, `"not_found"`, false},
		{"/healthz", 200, "ok", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			w := get(tt.path)
			if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.body) {
				t.Fatalf("%s: %d %q", tt.path, w.Code, w.Body.String())
			}
			if hasCSP := w.Header().Get("Content-Security-Policy") != ""; hasCSP != tt.csp {
				t.Fatalf("%s: CSP present = %v, want %v", tt.path, hasCSP, tt.csp)
			}
		})
	}

	t.Run("without WEB_DIR only the API is served", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do(http.MethodGet, "/", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})
}
