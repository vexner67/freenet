package rest_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

type batchResponse struct {
	Responses []struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	} `json:"responses"`
}

func TestBatch_AnswersEachReadAsItself(t *testing.T) {
	t.Parallel()

	// The reads go at once, so each is of another fake service.
	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/batch",
		`{"requests":[{"path":"/api/v1/me"},{"path":"/api/v1/notifications/unread-count?x=1"},{"path":"/api/v1/nothing"}]}`,
		"Authorization", "Bearer tok")

	got := decode[batchResponse](t, w)
	if w.Code != http.StatusOK || len(got.Responses) != 3 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	// Each read goes with the caller's token and answers in its place.
	me, unread, nothing := got.Responses[0], got.Responses[1], got.Responses[2]
	if me.Status != http.StatusOK || !strings.Contains(string(me.Body), `"nickname"`) || h.users.token != "tok" {
		t.Fatalf("me: %d %s, token %q", me.Status, me.Body, h.users.token)
	}

	if unread.Status != http.StatusOK || !strings.Contains(string(unread.Body), "3") || h.notifications.token != "tok" {
		t.Fatalf("unread: %d %s, token %q", unread.Status, unread.Body, h.notifications.token)
	}

	if nothing.Status != http.StatusNotFound {
		t.Fatalf("no route: %d %s", nothing.Status, nothing.Body)
	}
}

func TestBatch_AnswersAPageAndAnError(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]string{
		"/api/v1/users/dina":   `"dina"`,
		"/api/v1/users/nobody": `"not_found"`,
	} {
		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/batch", `{"requests":[{"path":"`+path+`"}]}`)

		got := decode[batchResponse](t, w)
		if w.Code != http.StatusOK || len(got.Responses) != 1 || !strings.Contains(string(got.Responses[0].Body), want) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
}

func TestBatch_TakesOnlyReadsOfTheAPI(t *testing.T) {
	t.Parallel()

	tooMany := `{"requests":[` + strings.TrimSuffix(strings.Repeat(`{"path":"/api/v1/me"},`, 21), ",") + `]}`

	for name, body := range map[string]string{
		"none":     `{"requests":[]}`,
		"too many": tooMany,
		"site":     `{"requests":[{"path":"/feed"}]}`,
		"absolute": `{"requests":[{"path":"http://example.com/api/v1/me"}]}`,
		"relative": `{"requests":[{"path":"api/v1/me"}]}`,
		"batch":    `{"requests":[{"path":"/api/v1/batch"}]}`,
		"events":   `{"requests":[{"path":"/api/v1/events"}]}`,
	} {
		h := newHarness(rest.Config{})
		if w := h.do("POST", "/api/v1/batch", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
}
