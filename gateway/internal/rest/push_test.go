package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestPushKey(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	// Pushes off: no key, and anyone may ask.
	w := h.do("GET", "/api/v1/push/key", "")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["publicKey"] != nil {
		t.Fatalf("%d %v", w.Code, got)
	}

	h.notifications.pushKey = "BKey"
	w = h.do("GET", "/api/v1/push/key", "")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["publicKey"] != "BKey" {
		t.Fatalf("%d %v", w.Code, got)
	}
}

func TestSubscribePush(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	body := `{"endpoint":"https://push.example/1","expirationTime":null,"keys":{"p256dh":"BKey","auth":"secret"}}`

	if w := h.do("PUT", "/api/v1/me/push", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	w := h.do("PUT", "/api/v1/me/push", body, "Authorization", "Bearer tok")
	if s := h.notifications.subscribed; w.Code != http.StatusNoContent || s.Endpoint != "https://push.example/1" || s.P256DH != "BKey" || s.Auth != "secret" {
		t.Fatalf("%d %+v", w.Code, s)
	}

	if w = h.do("PUT", "/api/v1/me/push", `{"keys":{}}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("without an endpoint: %d", w.Code)
	}

	w = h.do("DELETE", "/api/v1/me/push", `{"endpoint":"https://push.example/1"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.notifications.unsubscribed != "https://push.example/1" {
		t.Fatalf("unsubscribe: %d %q", w.Code, h.notifications.unsubscribed)
	}
}
