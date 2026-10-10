package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type messageJSON struct {
	ID             string `json:"id"`
	Text           string `json:"text"`
	CreatedAt      string `json:"createdAt"`
	Out            bool   `json:"out"`
	Read           bool   `json:"read"`
	DeletableUntil string `json:"deletableUntil"`
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
	if got.Messages[0].DeletableUntil != "2026-10-06T22:00:00Z" || got.Messages[1].DeletableUntil != "" {
		t.Fatalf("messages = %+v; only ann's own can be deleted", got.Messages)
	}

	poll := newHarness(rest.Config{})
	if w = poll.do("GET", "/api/v1/dialogs/bob/messages?after=m2", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || poll.messages.after != "m2" {
		t.Fatalf("polling: %d, after %q", w.Code, poll.messages.after)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/dialogs/nobody/messages", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}

	// Around a message: the dialog as a found message opens it, with
	// whether the caller may write, and a message taken back in its place.
	found := newHarness(rest.Config{})
	w = found.do("GET", "/api/v1/dialogs/bob/messages?around=m2", "", "Authorization", "Bearer tok")
	page := decode[struct {
		Messages []struct {
			ID             string `json:"id"`
			Text           string `json:"text"`
			Deleted        bool   `json:"deleted"`
			DeletableUntil string `json:"deletableUntil"`
			EditableUntil  string `json:"editableUntil"`
		} `json:"messages"`
		CanWrite bool `json:"canWrite"`
	}](t, w)
	if w.Code != http.StatusOK || found.messages.around != "m2" || !page.CanWrite || len(page.Messages) != 3 {
		t.Fatalf("around: %d %s", w.Code, w.Body)
	}
	if gone := page.Messages[2]; !gone.Deleted || gone.Text != "" || gone.DeletableUntil != "" || gone.EditableUntil != "" || page.Messages[0].Deleted {
		t.Fatalf("messages = %+v; want the last taken back and nothing to do with it", page.Messages)
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

	t.Run("forwarding messages", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"text":"вот","forward":["m1","gone"]}`, "Authorization", "Bearer tok")
		got := decode[struct {
			Text           string `json:"text"`
			ForwardedCount int    `json:"forwardedCount"`
			Forwarded      []struct {
				Author    map[string]string   `json:"author"`
				Text      string              `json:"text"`
				Photos    []map[string]string `json:"photos"`
				CreatedAt string              `json:"createdAt"`
			} `json:"forwarded"`
		}](t, w)

		// Both are counted; the one whose author's page is gone is not shown.
		if w.Code != http.StatusCreated || got.Text != "вот" || got.ForwardedCount != 2 || len(got.Forwarded) != 1 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if f := got.Forwarded[0]; f.Author["nickname"] != "bob" || f.Text != "из m1" || f.CreatedAt != "2026-10-09T12:00:00Z" ||
			len(f.Photos) != 1 || f.Photos[0]["large"] != "/media/photos/u2/ph1/large.jpg" {
			t.Fatalf("forwarded = %+v", f)
		}
	})

	t.Run("with a shared post", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"post":"p1"}`, "Authorization", "Bearer tok")
		got := decode[struct {
			Post   string `json:"post"`
			Repost *struct {
				ID     string            `json:"id"`
				Text   string            `json:"text"`
				Author map[string]string `json:"author"`
			} `json:"repost"`
		}](t, w)
		if w.Code != http.StatusCreated || got.Post != "p1" || got.Repost == nil || got.Repost.Text != "первая" || got.Repost.Author["nickname"] != "ann" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}

		if w = h.do("POST", "/api/v1/dialogs/bob/messages", `{"post":"p404"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Fatalf("a missing post: %d", w.Code)
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

func TestDeleteMessage(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("DELETE", "/api/v1/messages/m1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.messages.token != "tok" || h.messages.deleted != "m1" {
		t.Fatalf("%d %+v", w.Code, h.messages)
	}

	if w := newHarness(rest.Config{}).do("DELETE", "/api/v1/messages/m1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	for kind, want := range map[failure.Kind]int{
		failure.PermissionDenied: http.StatusForbidden,
		failure.Conflict:         http.StatusConflict,
		failure.NotFound:         http.StatusNotFound,
	} {
		h = newHarness(rest.Config{})
		h.messages.err = failure.New(kind, "no")
		if w := h.do("DELETE", "/api/v1/messages/m1", "", "Authorization", "Bearer tok"); w.Code != want {
			t.Errorf("%v: %d, want %d", kind, w.Code, want)
		}
	}
}

func TestEditHideAndSearchMessages(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	// "Удалить у себя": for=me hides, nothing deletes for both.
	if w := h.do("DELETE", "/api/v1/messages/m1?for=me", "", auth...); w.Code != http.StatusNoContent || h.messages.hidden != "m1" || h.messages.deleted != "" {
		t.Fatalf("hide: %d, hidden %q, deleted %q", w.Code, h.messages.hidden, h.messages.deleted)
	}
	if w := h.do("DELETE", "/api/v1/messages/m1?for=all", "", auth...); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad for: %d", w.Code)
	}
	if w := h.do("DELETE", "/api/v1/messages/m1?for=me", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("hide without a token: %d", w.Code)
	}

	// Edit: the message as the caller's own, with when it was edited.
	w := h.do("PATCH", "/api/v1/messages/m1", `{"text":"привет!"}`, auth...)
	edited := decode[struct {
		Text          string  `json:"text"`
		Out           bool    `json:"out"`
		EditedAt      *string `json:"editedAt"`
		EditableUntil string  `json:"editableUntil"`
	}](t, w)
	if w.Code != http.StatusOK || h.messages.edited != "m1/привет!" || !edited.Out || edited.EditedAt == nil || edited.EditableUntil == "" {
		t.Fatalf("edit: %d %+v, edited %q", w.Code, edited, h.messages.edited)
	}
	if w = h.do("PATCH", "/api/v1/messages/m1", `{"text":"x"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("edit without a token: %d", w.Code)
	}

	// Search: all dialogs, each message with who is on the other side.
	w = h.do("GET", "/api/v1/messages/search?q=%D0%BC%D0%BE%D1%80%D0%B5&cursor=m10", "", auth...)
	found := decode[struct {
		Messages []struct {
			Peer    map[string]any `json:"peer"`
			Message struct {
				ID  string `json:"id"`
				Out bool   `json:"out"`
			} `json:"message"`
		} `json:"messages"`
		NextCursor string `json:"nextCursor"`
	}](t, w)
	if w.Code != http.StatusOK || h.messages.searched != "море" || h.messages.peerID != "" || h.messages.cursor != "m10" || found.NextCursor != "m8" ||
		len(found.Messages) == 0 || found.Messages[0].Peer["nickname"] != "bob" || found.Messages[0].Message.ID != "m9" || found.Messages[0].Message.Out {
		t.Fatalf("search: %d %+v, asked for %q with %q", w.Code, found, h.messages.searched, h.messages.peerID)
	}

	// In one dialog: the peer by nickname.
	if w = h.do("GET", "/api/v1/messages/search?q=x&peer=bob", "", auth...); w.Code != http.StatusOK || h.messages.peerID != "u2" {
		t.Fatalf("search with bob: %d, peer %q", w.Code, h.messages.peerID)
	}
	if w = h.do("GET", "/api/v1/messages/search?q=x&peer=nobody", "", auth...); w.Code != http.StatusNotFound {
		t.Fatalf("search with nobody: %d", w.Code)
	}
	if w = h.do("GET", "/api/v1/messages/search?q=x", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("search without a token: %d", w.Code)
	}
}
