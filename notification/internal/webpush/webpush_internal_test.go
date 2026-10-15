package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/notification/internal/service"
)

func b64(t *testing.T, s string) []byte {
	t.Helper()

	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// TestSeal_RFC8291 encrypts the example of RFC 8291, appendix A.
func TestSeal_RFC8291(t *testing.T) {
	t.Parallel()

	server, err := ecdh.P256().NewPrivateKey(b64(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}

	keys := push.Keys{
		P256DH: b64(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		Auth:   b64(t, "BTBZMqHH6r4Tts7J_aSIgg"),
	}

	got, err := seal([]byte("When I grow up, I want to be a watermelon"), keys, server, b64(t, "DGv6ra1nlYgDCS1FRnbzlw"))
	if err != nil {
		t.Fatal(err)
	}

	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if s := base64.RawURLEncoding.EncodeToString(got); s != want {
		t.Fatalf("got\n%s\nwant\n%s", s, want)
	}
}

func TestSend(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	sender, err := New(key, "https://freenet.example", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var got *http.Request

	status := http.StatusCreated
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	sender.client = srv.Client()

	browser, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	sub := push.RestoreSubscription(uuid.New(), uuid.New(), push.Endpoint(srv.URL+"/push/1"),
		push.Keys{P256DH: browser.PublicKey().Bytes(), Auth: make([]byte, 16)}, time.Now())

	if err = sender.Send(context.Background(), sub, push.Payload{Type: push.TypeMessage, ID: uuid.New()}); err != nil {
		t.Fatal(err)
	}

	if got.Header.Get("Content-Encoding") != "aes128gcm" || got.Header.Get("Urgency") != "high" || got.Header.Get("TTL") != "86400" {
		t.Fatalf("headers %v", got.Header)
	}

	// The token is signed by the key browsers subscribed with, for the
	// origin of the endpoint.
	auth := got.Header.Get("Authorization")
	token, pub, ok := strings.Cut(strings.TrimPrefix(auth, "vapid t="), ", k=")
	if !ok || pub != sender.PublicKey() {
		t.Fatalf("Authorization %q", auth)
	}

	claims := jwt.MapClaims{}
	if _, err = jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
		jwt.WithValidMethods([]string{"ES256"}), jwt.WithAudience(srv.URL)); err != nil {
		t.Fatalf("token: %v", err)
	}

	status = http.StatusGone
	if err = sender.Send(context.Background(), sub, push.Payload{Type: push.TypeNotification, ID: uuid.New()}); !errors.Is(err, service.ErrSubscriptionGone) {
		t.Fatalf("410: %v", err)
	}

	status = http.StatusTooManyRequests
	if err = sender.Send(context.Background(), sub, push.Payload{Type: push.TypeNotification, ID: uuid.New()}); err == nil || strings.Contains(err.Error(), "/push/1") {
		t.Fatalf("429: %v", err)
	}

	// Nor does an error of the connection.
	srv.Close()
	if err = sender.Send(context.Background(), sub, push.Payload{Type: push.TypeNotification, ID: uuid.New()}); err == nil || strings.Contains(err.Error(), "/push/1") {
		t.Fatalf("closed: %v", err)
	}
}
