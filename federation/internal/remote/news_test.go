package remote_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/remote"
)

func TestFinger(t *testing.T) {
	t.Parallel()

	var resource string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resource = r.URL.Query().Get("resource")

		switch {
		case r.URL.Path != "/.well-known/webfinger":
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(resource, "acct:ann@"):
			_, _ = io.WriteString(w, `{"links":[
				{"rel":"http://webfinger.net/rel/profile-page","type":"text/html","href":"https://m.example/@ann"},
				{"rel":"self","type":"application/activity+json","href":"https://m.example/users/ann"}]}`)
		case strings.HasPrefix(resource, "acct:nolink@"):
			_, _ = io.WriteString(w, `{"links":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := remote.NewForTest(testSite(t), srv)
	host := hostOf(t, srv)

	id, err := c.Finger(context.Background(), "ann", host)
	if err != nil || id != "https://m.example/users/ann" {
		t.Fatalf("Finger = %q, %v", id, err)
	}

	if resource != "acct:ann@"+host {
		t.Errorf("resource %q", resource)
	}

	for _, username := range []string{"nolink", "nobody"} {
		if _, err := c.Finger(context.Background(), username, host); !errors.Is(err, fedi.ErrNotFound) {
			t.Errorf("Finger %s = %v, want ErrNotFound", username, err)
		}
	}
}

func TestFetch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cat.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "png")
		case "/big.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, 8<<20+1))
		case "/evil.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = io.WriteString(w, "<svg/>")
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<p>")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := remote.NewForTest(testSite(t), srv)

	m, err := c.Fetch(context.Background(), fedi.URL(srv.URL+"/cat.png"))
	if err != nil || string(m.Body) != "png" || m.ContentType != "image/png" {
		t.Fatalf("Fetch = %+v, %v", m, err)
	}

	for _, path := range []string{"/big.png", "/evil.svg", "/page", "/missing.png"} {
		if _, err := c.Fetch(context.Background(), fedi.URL(srv.URL+path)); !errors.Is(err, fedi.ErrNotFound) {
			t.Errorf("Fetch %s = %v, want ErrNotFound", path, err)
		}
	}
}

func testSite(t *testing.T) fedi.Site {
	t.Helper()

	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return site
}

func hostOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	return u.Host
}
