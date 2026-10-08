package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type messageJSON struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
	Out       bool   `json:"out"`
	Read      bool   `json:"read"`
}

func TestListDialogs(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/dialogs?cursor=m5", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Dialogs []struct {
			Peer        map[string]string `json:"peer"`
			LastMessage messageJSON       `json:"lastMessage"`
			Unread      bool              `json:"unread"`
		} `json:"dialogs"`
		NextCursor string `json:"nextCursor"`
	}](t, w)

	if w.Code != http.StatusOK || got.NextCursor != "m1" || h.messages.token != "tok" || h.messages.cursor != "m5" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	// The dialog with a vanished account is left out.
	if len(got.Dialogs) != 1 {
		t.Fatalf("dialogs = %+v", got.Dialogs)
	}
	if d := got.Dialogs[0]; d.Peer["nickname"] != "bob" || !d.Unread || d.LastMessage.Out || d.LastMessage.Text != "как дела?" {
		t.Fatalf("dialog = %+v", d)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/dialogs", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestCountUnreadDialogs(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/dialogs/unread-count", "", "Authorization", "Bearer tok")

	if got := decode[map[string]int](t, w); w.Code != http.StatusOK || got["count"] != 2 || h.messages.token != "tok" {
		t.Fatalf("%d %v", w.Code, got)
	}
}

func TestListMessages(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/dialogs/bob/messages?cursor=m7", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Peer         map[string]string `json:"peer"`
		Messages     []messageJSON     `json:"messages"`
		NextCursor   string            `json:"nextCursor"`
		PeerReadUpTo string            `json:"peerReadUpTo"`
	}](t, w)

	if w.Code != http.StatusOK || got.Peer["nickname"] != "bob" || got.NextCursor != "m1" || got.PeerReadUpTo != "m1" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.messages.peerID != "u2" || h.messages.cursor != "m7" || h.messages.after != "" {
		t.Fatalf("request = %+v", h.messages)
	}
	if len(got.Messages) != 2 || !got.Messages[0].Out || !got.Messages[0].Read || got.Messages[1].Out {
		t.Fatalf("messages = %+v; ann's own first, then bob's", got.Messages)
	}

	poll := newHarness(rest.Config{})
	if w = poll.do("GET", "/api/v1/dialogs/bob/messages?after=m2", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || poll.messages.after != "m2" {
		t.Fatalf("polling: %d, after %q", w.Code, poll.messages.after)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/dialogs/nobody/messages", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
}

func TestSendMessage(t *testing.T) {
	t.Parallel()

	t.Run("to a user by nickname", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"text":"привет"}`, "Authorization", "Bearer tok")
		got := decode[messageJSON](t, w)

		if w.Code != http.StatusCreated || h.messages.peerID != "u2" || h.messages.sentText != "привет" || h.messages.token != "tok" {
			t.Fatalf("%d %+v", w.Code, got)
		}
		if !got.Out || got.Read || got.CreatedAt != "2026-10-05T22:00:00Z" {
			t.Fatalf("message = %+v", got)
		}
	})

	t.Run("with photos, the caller's only", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"photos":["ph1","ph2"]}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)
		if photos, _ := got["photos"].([]any); w.Code != http.StatusCreated || len(photos) != 2 {
			t.Fatalf("%d %v", w.Code, got)
		}

		h = newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		if w = h.do("POST", "/api/v1/dialogs/bob/messages", `{"photos":["ph7"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Fatalf("someone else's photo: %d", w.Code)
		}
	})

	t.Run("profile must be filled in first", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"text":"привет"}`, "Authorization", "Bearer tok")

		if got := decode[errorBody](t, w); w.Code != http.StatusBadRequest || got.Error.Message != "fill in your profile before writing messages" || h.messages.sentText != "" {
			t.Fatalf("%d %+v", w.Code, got)
		}
	})

	t.Run("to nobody", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		if w := h.do("POST", "/api/v1/dialogs/nobody/messages", `{"text":"привет"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("needs a token", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("POST", "/api/v1/dialogs/bob/messages", `{"text":"x"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestMarkDialogRead(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/dialogs/bob/read", `{"upTo":"m2"}`, "Authorization", "Bearer tok")

	if w.Code != http.StatusNoContent || h.messages.peerID != "u2" || h.messages.markedUpTo != "m2" {
		t.Fatalf("%d %+v", w.Code, h.messages)
	}

	h = newHarness(rest.Config{})
	h.messages.err = failure.New(failure.NotFound, "message not found")
	if w = h.do("POST", "/api/v1/dialogs/bob/read", `{"upTo":"someone-else"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("not a message from bob: %d", w.Code)
	}
}
