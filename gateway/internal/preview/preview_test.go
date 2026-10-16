package preview_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/preview"
)

func TestPublic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
	}{
		{"93.184.216.34:443", true},
		{"93.184.216.34:80", true},
		{"[2606:2800:220:1::1]:443", true},
		{"93.184.216.34:22", false},
		{"127.0.0.1:80", false},
		{"10.1.2.3:80", false},
		{"172.16.0.1:443", false},
		{"192.168.1.1:80", false},
		{"169.254.169.254:80", false},
		{"100.64.0.1:80", false},
		{"0.0.0.0:80", false},
		{"255.255.255.255:80", false},
		{"224.0.0.1:80", false},
		{"[::1]:80", false},
		{"[::]:80", false},
		{"[::ffff:127.0.0.1]:80", false},
		{"[::ffff:10.0.0.1]:443", false},
		{"[64:ff9b::a00:1]:80", false},
		{"[2002:a00:1::]:80", false},
		{"[fd00::1]:80", false},
		{"[fe80::1]:80", false},
	}

	for _, tt := range tests {
		if got := preview.Public(netip.MustParseAddrPort(tt.addr)); got != tt.want {
			t.Errorf("Public(%s) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

// loopback lets the tests reach their own servers.
func loopback(netip.AddrPort) bool { return true }

func TestRead(t *testing.T) {
	t.Parallel()

	jpeg := append([]byte{0xff, 0xd8, 0xff}, bytes.Repeat([]byte{1}, 100)...)

	mux := http.NewServeMux()
	mux.HandleFunc("/og", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Plain</title>
			<meta property="og:title" content="  Кошки
			и  коты ">
			<meta name="description" content="Plain description">
			<meta property="og:description" content="Всё о кошках &amp; котах">
			<meta property="og:site_name" content="Котопедия">
			<meta property="og:image" content="/pic.jpg">
			</head><body><meta property="og:title" content="from the body"></body></html>`))
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=windows-1251")
		// "Привет" in windows-1251.
		_, _ = w.Write(append([]byte("<title>"), append([]byte{0xcf, 0xf0, 0xe8, 0xe2, 0xe5, 0xf2}, []byte("</title>")...)...))
	})
	mux.HandleFunc("/pic.jpg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpeg)
	})
	mux.HandleFunc("/huge", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Huge</title><meta property="og:image" content="/huge.jpg">`))
	})
	mux.HandleFunc("/huge.jpg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(make([]byte, 6<<20))
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"no"}`))
	})
	mux.HandleFunc("/untitled", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<p>nothing</p>`))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/og", http.StatusFound)
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	r := preview.NewAllowing(2*time.Second, loopback)
	ctx := context.Background()

	t.Run("open graph", func(t *testing.T) {
		t.Parallel()

		p, err := r.Read(ctx, srv.URL+"/moved")
		if err != nil {
			t.Fatal(err)
		}

		if p.URL != srv.URL+"/moved" || p.Title != "Кошки и коты" || p.Description != "Всё о кошках & котах" ||
			p.Site != "Котопедия" || !bytes.Equal(p.Image, jpeg) {
			t.Fatalf("preview = %+v", p)
		}
	})

	t.Run("plain title in another charset, the host for the site", func(t *testing.T) {
		t.Parallel()

		p, err := r.Read(ctx, srv.URL+"/plain")
		if err != nil {
			t.Fatal(err)
		}

		if p.Title != "Привет" || p.Description != "" || p.Site != "127.0.0.1" || p.Image != nil {
			t.Fatalf("preview = %+v", p)
		}
	})

	t.Run("a picture too big is left out", func(t *testing.T) {
		t.Parallel()

		p, err := r.Read(ctx, srv.URL+"/huge")
		if err != nil || p.Title != "Huge" || p.Image != nil {
			t.Fatalf("preview = %+v, %v", p, err)
		}
	})

	for _, path := range []string{"/json", "/untitled", "/none"} {
		t.Run("no preview at "+path, func(t *testing.T) {
			t.Parallel()

			if _, err := r.Read(ctx, srv.URL+path); !errors.Is(err, preview.ErrNoPreview) {
				t.Fatalf("err = %v", err)
			}
		})
	}

	for _, path := range []string{"/loop", "/file"} {
		t.Run("redirects of "+path+" are refused", func(t *testing.T) {
			t.Parallel()

			if _, err := r.Read(ctx, srv.URL+path); err == nil {
				t.Fatal("read")
			}
		})
	}

	t.Run("only web addresses", func(t *testing.T) {
		t.Parallel()

		for _, u := range []string{"ftp://example.com/", "file:///etc/passwd", "http://user:pass@" + strings.TrimPrefix(srv.URL, "http://") + "/og", "http:///og"} {
			if _, err := r.Read(ctx, u); !errors.Is(err, preview.ErrForbidden) {
				t.Errorf("%s: err = %v", u, err)
			}
		}
	})

	t.Run("the inside network is out of reach", func(t *testing.T) {
		t.Parallel()

		// The server listens on loopback, which a real reader never dials,
		// however the name of the link resolves.
		public := preview.New(2 * time.Second)
		for _, u := range []string{srv.URL + "/og", strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/og"} {
			if _, err := public.Read(ctx, u); !errors.Is(err, preview.ErrForbidden) {
				t.Errorf("%s: err = %v", u, err)
			}
		}
	})
}
