package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeFederation struct {
	token    string
	open     bool
	resource string
	read     string
	received model.InboxRequest
	err      error
}

func (f *fakeFederation) status() model.Federation {
	return model.Federation{Open: f.open, Followers: 3, Domain: "freenet.example"}
}

func (f *fakeFederation) Federation(_ context.Context, token string) (model.Federation, error) {
	f.token = token
	return f.status(), f.err
}

func (f *fakeFederation) Open(_ context.Context, token string) (model.Federation, error) {
	f.token, f.open = token, true
	return f.status(), f.err
}

func (f *fakeFederation) Close(_ context.Context, token string) (model.Federation, error) {
	f.token, f.open = token, false
	return f.status(), f.err
}

func (f *fakeFederation) Finger(_ context.Context, resource string) ([]byte, error) {
	f.resource = resource
	return []byte(`{"subject":"acct:anya@freenet.example"}`), f.err
}

func (f *fakeFederation) Actor(_ context.Context, id string) (model.FederationDocument, error) {
	f.read = "actor " + id
	return model.FederationDocument{Body: []byte(`{"type":"Person"}`), PageURL: "https://freenet.example/anya"}, f.err
}

func (f *fakeFederation) Note(_ context.Context, id string) (model.FederationDocument, error) {
	f.read = "note " + id
	return model.FederationDocument{Body: []byte(`{"type":"Note"}`), PageURL: "https://freenet.example/post/" + id}, f.err
}

func (f *fakeFederation) Followers(_ context.Context, id string) ([]byte, error) {
	f.read = "followers " + id
	return []byte(`{"totalItems":3}`), f.err
}

func (f *fakeFederation) Outbox(_ context.Context, id string) ([]byte, error) {
	f.read = "outbox " + id
	return []byte(`{"totalItems":0}`), f.err
}

func (f *fakeFederation) Receive(_ context.Context, in model.InboxRequest) error {
	f.received = in
	return f.err
}

func TestFederationSettings(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	if w := h.do("GET", "/api/v1/me/federation", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	w := h.do("PUT", "/api/v1/me/federation", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.federation.token != "tok" {
		t.Fatalf("open: %d %s", w.Code, w.Body)
	}

	got := decode[struct {
		Open      bool   `json:"open"`
		Followers int    `json:"followers"`
		Domain    string `json:"domain"`
	}](t, w)
	if !got.Open || got.Followers != 3 || got.Domain != "freenet.example" {
		t.Fatalf("open = %+v", got)
	}

	if w = h.do("DELETE", "/api/v1/me/federation", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.federation.open {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}

	h.federation.err = failure.New(failure.Conflict, "the page is not open to everyone")
	if w = h.do("PUT", "/api/v1/me/federation", "", "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("hidden page: %d", w.Code)
	}
}

func TestWebFinger(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/.well-known/webfinger?resource=acct:anya@freenet.example", "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/jrd+json" || h.federation.resource != "acct:anya@freenet.example" {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Content-Type"), h.federation.resource)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("headers = %v", w.Header())
	}

	if w = h.do("GET", "/.well-known/webfinger", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("without a resource: %d", w.Code)
	}

	h.federation.err = failure.New(failure.NotFound, "not found")
	if w = h.do("GET", "/.well-known/webfinger?resource=acct:boris@freenet.example", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", w.Code)
	}
}

func TestFederationDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		accept   string
		code     int
		read     string
		location string
	}{
		{"actor", "/ap/users/u1", "application/activity+json", http.StatusOK, "actor u1", ""},
		{"actor for a browser", "/ap/users/u1", "text/html,application/xhtml+xml", http.StatusFound, "actor u1", "https://freenet.example/anya"},
		{"note asked by Mastodon", "/ap/posts/p1", `application/ld+json; profile="https://www.w3.org/ns/activitystreams", text/html`, http.StatusOK, "note p1", ""},
		{"note for a browser", "/ap/posts/p1", "text/html", http.StatusFound, "note p1", "https://freenet.example/post/p1"},
		{"followers", "/ap/users/u1/followers", "text/html", http.StatusOK, "followers u1", ""},
		{"outbox", "/ap/users/u1/outbox", "", http.StatusOK, "outbox u1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})

			w := h.do("GET", tt.path, "", "Accept", tt.accept)
			if w.Code != tt.code || h.federation.read != tt.read || w.Header().Get("Location") != tt.location {
				t.Fatalf("%d, read %q, location %q", w.Code, h.federation.read, w.Header().Get("Location"))
			}
			if tt.code == http.StatusOK && w.Header().Get("Content-Type") != "application/activity+json" {
				t.Fatalf("Content-Type = %q", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestInbox(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("POST", "/ap/users/u1/inbox?x=1", `{"type":"Follow"}`,
		"Content-Type", "application/activity+json", "Signature", `keyId="https://m.example/u#k"`, "Date", "Mon, 02 Nov 2026 12:00:00 GMT")
	if w.Code != http.StatusAccepted {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	got := h.federation.received
	if got.Method != "POST" || got.Path != "/ap/users/u1/inbox?x=1" || got.UserID != "u1" || string(got.Body) != `{"type":"Follow"}` {
		t.Fatalf("received = %+v", got)
	}
	if got.Headers["signature"] != `keyId="https://m.example/u#k"` || got.Headers["host"] != "example.com" || got.Headers["content-type"] != "application/activity+json" {
		t.Fatalf("headers = %v", got.Headers)
	}

	if w = h.do("POST", "/ap/inbox", `{}`); w.Code != http.StatusAccepted || h.federation.received.UserID != "" {
		t.Fatalf("shared inbox: %d %+v", w.Code, h.federation.received)
	}

	if w = h.do("POST", "/ap/inbox", strings.Repeat("x", 300<<10)); w.Code != http.StatusBadRequest {
		t.Fatalf("too large: %d", w.Code)
	}

	h.federation.err = failure.New(failure.Unauthenticated, "bad signature")
	if w = h.do("POST", "/ap/inbox", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", w.Code)
	}
}

func TestFederationOff(t *testing.T) {
	t.Parallel()

	handler := mustServer(rest.Config{RequestTimeout: time.Second}, rest.Backends{}, prometheus.NewRegistry()).Handler()

	for _, path := range []string{"/api/v1/me/federation", "/.well-known/webfinger?resource=acct:anya@freenet.example", "/ap/users/u1"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))

		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, w.Code)
		}
	}
}
