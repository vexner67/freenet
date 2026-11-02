package remote_test

import (
	"context"
	"errors"
	"fmt"
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

func TestVideoHead(t *testing.T) {
	t.Parallel()

	var ranges []string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))

		switch r.URL.Path {
		case "/part.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "mp4")
		case "/whole.mp4":
			// A server that ignores the range sends all of it.
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(make([]byte, remote.VideoHeadSize+100))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<p>")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := remote.NewForTest(testSite(t), srv)

	head, err := c.VideoHead(context.Background(), fedi.URL(srv.URL+"/part.mp4"))
	if err != nil || string(head) != "mp4" {
		t.Fatalf("VideoHead = %q, %v", head, err)
	}

	if want := fmt.Sprintf("bytes=0-%d", remote.VideoHeadSize-1); ranges[0] != want {
		t.Errorf("range %q, want %q", ranges[0], want)
	}

	if head, err = c.VideoHead(context.Background(), fedi.URL(srv.URL+"/whole.mp4")); err != nil || len(head) != remote.VideoHeadSize {
		t.Fatalf("VideoHead of a whole video = %d bytes, %v; want its head", len(head), err)
	}

	for _, path := range []string{"/page", "/missing.mp4"} {
		if _, err := c.VideoHead(context.Background(), fedi.URL(srv.URL+path)); !errors.Is(err, fedi.ErrNotFound) {
			t.Errorf("VideoHead %s = %v, want ErrNotFound", path, err)
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
