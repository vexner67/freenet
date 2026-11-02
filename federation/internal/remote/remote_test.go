package remote_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/httpsig"
	"github.com/vexner67/freenet/federation/internal/remote"
)

func TestPublic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
	}{
		{"1.1.1.1:443", true},
		{"[2606:4700::1111]:443", true},
		{"1.1.1.1:80", false},
		{"127.0.0.1:443", false},
		{"10.0.0.1:443", false},
		{"172.31.250.10:443", false},
		{"192.168.1.1:443", false},
		{"100.64.0.1:443", false},
		{"169.254.169.254:443", false},
		{"[::1]:443", false},
		{"[::ffff:10.0.0.1]:443", false},
		{"[64:ff9b::a00:1]:443", false},
	}

	for _, tt := range tests {
		if got := remote.Public(netip.MustParseAddrPort(tt.addr)); got != tt.want {
			t.Errorf("Public(%s) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func newAccount(t *testing.T) *fedi.Account {
	t.Helper()

	keys, err := httpsig.Keys{}.NewKeys()
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}

	a, err := fedi.NewAccount(uuid.New(), keys, time.Now())
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}

	return a
}

func TestActor(t *testing.T) {
	t.Parallel()

	var signature string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature = r.Header.Get("Signature")

		switch r.URL.Path {
		case "/users/anya":
			_, _ = io.WriteString(w, `{"id":"x","inbox":"y","publicKey":{"id":"z","publicKeyPem":"PEM"}}`)
		case "/users/gone":
			w.WriteHeader(http.StatusGone)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	c := remote.NewForTest(site, srv)
	signer := newAccount(t)

	doc, err := c.Actor(context.Background(), fedi.URL(srv.URL+"/users/anya"), signer)
	if err != nil || doc.PublicKey != "PEM" {
		t.Fatalf("Actor = %+v, %v", doc, err)
	}

	if !strings.Contains(signature, string(site.Key(signer.UserID()))) {
		t.Fatalf("the request is signed %q, want with the key of the signer", signature)
	}

	if _, err = c.Actor(context.Background(), fedi.URL(srv.URL+"/users/gone"), nil); !errors.Is(err, fedi.ErrGone) {
		t.Fatalf("Actor of a gone actor = %v", err)
	}

	var status remote.StatusError
	if _, err = c.Actor(context.Background(), fedi.URL(srv.URL+"/users/broken"), nil); !errors.As(err, &status) || status.Code != 500 {
		t.Fatalf("Actor of a broken server = %v", err)
	}
}

func TestPost(t *testing.T) {
	t.Parallel()

	var got []byte

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/activity+json" || r.Header.Get("Digest") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	c := remote.NewForTest(site, srv)

	if err = c.Post(context.Background(), fedi.URL(srv.URL+"/inbox"), []byte(`{"type":"Create"}`), newAccount(t)); err != nil {
		t.Fatalf("Post: %v", err)
	}

	if string(got) != `{"type":"Create"}` {
		t.Fatalf("the inbox got %q", got)
	}
}

// The real client never dials loopback, whatever the name resolves to.
func TestNoInsideAddresses(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	if _, err = remote.New(site, time.Second).Actor(context.Background(), fedi.URL(srv.URL+"/users/anya"), nil); !errors.Is(err, remote.ErrForbidden) {
		t.Fatalf("Actor on loopback = %v, want ErrForbidden", err)
	}
}
