package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestBlacklist(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/me/blacklist/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.users.blocked != "u2" {
		t.Fatalf("block: %d, blocked %q", w.Code, h.users.blocked)
	}
	if w := h.do("DELETE", "/api/v1/me/blacklist/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.users.unblocked != "u2" {
		t.Fatalf("unblock: %d, unblocked %q", w.Code, h.users.unblocked)
	}

	w := h.do("GET", "/api/v1/me/blacklist?cursor=c1", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	users, _ := got["users"].([]any)
	if w.Code != http.StatusOK || got["total"] != 1.0 || len(users) != 1 || h.users.cursor != "c1" {
		t.Fatalf("list: %d %v", w.Code, got)
	}

	if w = h.do("PUT", "/api/v1/me/blacklist/nobody", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown nickname: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/me/blacklist", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestGetProfile_Relation(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.relation = model.Relation{BlockedMe: true}

	got := decode[map[string]any](t, h.do("GET", "/api/v1/users/bob", "", "Authorization", "Bearer tok"))
	if got["blockedMe"] != true || got["blockedByMe"] != false || h.users.relationWith != "u2" {
		t.Fatalf("profile seen by a blacklisted viewer = %v", got)
	}

	anonymous := newHarness(rest.Config{})
	anonymous.users.relation = model.Relation{BlockedMe: true}
	got = decode[map[string]any](t, anonymous.do("GET", "/api/v1/users/bob", ""))
	if got["blockedMe"] != false || anonymous.users.relationWith != "" {
		t.Fatalf("anonymous viewers have no blacklist: %v", got)
	}
}

func TestSendMessage_Permissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		closed    bool
		peerWrote bool
		relation  model.Relation
		want      int
	}{
		{"open messages", false, false, model.Relation{}, http.StatusCreated},
		{"closed messages", true, false, model.Relation{}, http.StatusForbidden},
		{"closed, but the peer wrote first", true, true, model.Relation{}, http.StatusCreated},
		{"blacklisted by the peer", false, true, model.Relation{BlockedMe: true}, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})
			h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
			h.users.closedMessages, h.users.relation, h.messages.peerWrote = tt.closed, tt.relation, tt.peerWrote

			w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"cHJpdmV0"}`, "Authorization", "Bearer tok")
			if w.Code != tt.want || h.users.wroteFirst != tt.peerWrote {
				t.Fatalf("status = %d, want %d; wroteFirst passed = %v", w.Code, tt.want, h.users.wroteFirst)
			}
			if tt.want == http.StatusForbidden && h.messages.sent != "" {
				t.Fatal("a refused message was sent")
			}
		})
	}
}

func TestListMessages_CanWrite(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.closedMessages = true

	if got := decode[map[string]any](t, h.do("GET", "/api/v1/dialogs/bob/messages", "", "Authorization", "Bearer tok")); got["canWrite"] != false {
		t.Fatalf("closed messages: canWrite = %v", got["canWrite"])
	}

	h.messages.peerWrote = true
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/dialogs/bob/messages", "", "Authorization", "Bearer tok")); got["canWrite"] != true {
		t.Fatalf("an answer to the peer: canWrite = %v", got["canWrite"])
	}

	polled := newHarness(rest.Config{})
	if got := decode[map[string]any](t, polled.do("GET", "/api/v1/dialogs/bob/messages?after=m1", "", "Authorization", "Bearer tok")); got["canWrite"] != false || polled.users.wroteFirst {
		t.Fatalf("polls do not ask: %v", got)
	}
}

func TestCreateComment_Blacklist(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	h.users.relation = model.Relation{BlockedByMe: true}

	// p4 is bob's post on ann's wall: ann blacklisted bob.
	w := h.do("POST", "/api/v1/posts/p4/comments", `{"text":"x"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusForbidden || h.posts.created != "" {
		t.Fatalf("comment under a blacklisted author: %d", w.Code)
	}

	// p1 is ann's own post: nobody to ask.
	own := newHarness(rest.Config{})
	own.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	own.users.relation = model.Relation{BlockedMe: true}
	if w = own.do("POST", "/api/v1/posts/p1/comments", `{"text":"x"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || own.users.relationWith != "" {
		t.Fatalf("comment under her own post: %d, asked about %q", w.Code, own.users.relationWith)
	}
}
