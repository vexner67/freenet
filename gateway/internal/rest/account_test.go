package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestDeactivateAndReactivate(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do(http.MethodPost, "/api/v1/me/deactivate", `{"reason":"надоело"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK {
		t.Fatalf("deactivate: %d %s", w.Code, w.Body)
	}
	me := decode[map[string]any](t, w)
	if me["deactivated"] != true || me["purgeAt"] != "2027-05-07T12:00:00Z" {
		t.Fatalf("deactivated user = %v", me)
	}
	if h.users.reason != "надоело" || h.users.token != "tok" {
		t.Fatalf("passed reason %q, token %q", h.users.reason, h.users.token)
	}

	w = h.do(http.MethodPost, "/api/v1/me/reactivate", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK {
		t.Fatalf("reactivate: %d %s", w.Code, w.Body)
	}
	if me = decode[map[string]any](t, w); me["deactivated"] != false || me["purgeAt"] != nil {
		t.Fatalf("restored user = %v", me)
	}

	if w = h.do(http.MethodPost, "/api/v1/me/deactivate", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("deactivate without a token: %d", w.Code)
	}
}

// A deleted page shows names only: no wall; its owner stays in dialogs,
// marked.
func TestDeactivatedPage(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do(http.MethodGet, "/api/v1/users/cat", "")
	if p := decode[map[string]any](t, w); w.Code != http.StatusOK || p["deactivated"] != true || p["name"] != "Cat" {
		t.Fatalf("profile of a deleted page: %d %v", w.Code, p)
	}

	if w = h.do(http.MethodGet, "/api/v1/users/cat/posts", ""); w.Code != http.StatusForbidden {
		t.Fatalf("wall of a deleted page: %d %s", w.Code, w.Body)
	}

	h.messages.dialogs = []model.Dialog{{PeerID: "u3", LastMessage: model.Message{ID: "m3", SenderID: "u3", RecipientID: "u1", Text: "пока"}}}
	w = h.do(http.MethodGet, "/api/v1/dialogs", "", "Authorization", "Bearer tok")
	page := decode[struct {
		Dialogs []struct {
			Peer map[string]any `json:"peer"`
		} `json:"dialogs"`
	}](t, w)
	if len(page.Dialogs) != 1 || page.Dialogs[0].Peer["deactivated"] != true {
		t.Fatalf("dialogs = %v", page)
	}
}
