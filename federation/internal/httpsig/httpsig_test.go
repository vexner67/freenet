package httpsig_test

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/httpsig"
	"github.com/vexner67/freenet/federation/internal/service"
)

const keyID = fedi.URL("https://freenet.example/ap/users/1#main-key")

func newKeys(t *testing.T) fedi.KeyPair {
	t.Helper()

	keys, err := httpsig.Keys{}.NewKeys()
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}

	return keys
}

// signed is a POST signed as another server signs it, seen as it comes
// to the inbox.
func signed(t *testing.T, keys fedi.KeyPair, body []byte, at time.Time) service.SignedRequest {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, "https://m.example/inbox?x=1", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/activity+json")

	if err = httpsig.Sign(req, body, keyID, keys.Private, at); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	headers := map[string]string{}
	for name := range req.Header {
		headers[strings.ToLower(name)] = req.Header.Get(name)
	}

	return service.SignedRequest{Method: req.Method, Path: req.URL.RequestURI(), Headers: headers, Body: body}
}

func TestSignAndVerify(t *testing.T) {
	t.Parallel()

	keys := newKeys(t)
	now := time.Now()
	body := []byte(`{"type":"Follow"}`)

	r := signed(t, keys, body, now)

	id, err := httpsig.Signatures{}.KeyID(r)
	if err != nil || id != keyID {
		t.Fatalf("KeyID = %q, %v", id, err)
	}

	if err = (httpsig.Signatures{}).Verify(r, keys.Public, now.Add(time.Minute)); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	tests := []struct {
		name   string
		change func(r *service.SignedRequest)
		key    string
		at     time.Time
	}{
		{"another body", func(r *service.SignedRequest) { r.Body = []byte(`{"type":"Undo"}`) }, keys.Public, now},
		{"another path", func(r *service.SignedRequest) { r.Path = "/inbox" }, keys.Public, now},
		{"another host", func(r *service.SignedRequest) { r.Headers["host"] = "evil.example" }, keys.Public, now},
		{"another key", func(*service.SignedRequest) {}, newKeys(t).Public, now},
		{"old", func(*service.SignedRequest) {}, keys.Public, now.Add(13 * time.Hour)},
		{"no signature", func(r *service.SignedRequest) { delete(r.Headers, "signature") }, keys.Public, now},
		{"digest left out", func(r *service.SignedRequest) {
			r.Headers["signature"] = strings.Replace(r.Headers["signature"], " digest", "", 1)
		}, keys.Public, now},
		{"not a key", func(*service.SignedRequest) {}, "nonsense", now},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := signed(t, keys, body, now)
			tt.change(&r)

			if err := (httpsig.Signatures{}).Verify(r, tt.key, tt.at); !errors.Is(err, fedi.ErrBadSignature) {
				t.Fatalf("Verify = %v, want ErrBadSignature", err)
			}
		})
	}
}

func TestKeyIDMalformed(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"", `keyId=abc`, `keyId="http://a.example/k",signature="x"`, `signature="x"`} {
		r := service.SignedRequest{Headers: map[string]string{"signature": header}}
		if _, err := (httpsig.Signatures{}).KeyID(r); !errors.Is(err, fedi.ErrBadSignature) {
			t.Errorf("KeyID(%q) = %v, want ErrBadSignature", header, err)
		}
	}
}

func TestSignGet(t *testing.T) {
	t.Parallel()

	keys := newKeys(t)

	req, err := http.NewRequest(http.MethodGet, "https://m.example/users/anya", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	if err = httpsig.Sign(req, nil, keyID, keys.Private, time.Now()); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if req.Header.Get("Digest") != "" || !strings.Contains(req.Header.Get("Signature"), `headers="(request-target) host date"`) {
		t.Fatalf("a GET is signed as %q", req.Header.Get("Signature"))
	}

	if err = httpsig.Sign(req, nil, keyID, "not a key", time.Now()); err == nil {
		t.Fatal("Sign with no key succeeded")
	}
}
