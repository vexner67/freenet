package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type replyJSON struct {
	ID         string            `json:"id"`
	Author     map[string]string `json:"author"`
	Text       string            `json:"text"`
	PhotoCount int               `json:"photoCount"`
	Deleted    bool              `json:"deleted"`
}

type answerJSON struct {
	ReplyTo   *replyJSON `json:"replyTo"`
	Reactions []struct {
		UserID   string `json:"userId"`
		Reaction string `json:"reaction"`
	} `json:"reactions"`
}

func TestReply(t *testing.T) {
	t.Parallel()

	t.Run("in a dialog", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"aWR1","replyTo":"m2"}`, "Authorization", annAuth)
		got := decode[answerJSON](t, w)

		if w.Code != http.StatusCreated || got.ReplyTo == nil {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if q := got.ReplyTo; q.ID != "m2" || q.Author["nickname"] != "bob" || q.PhotoCount != 1 || q.Text != "" {
			t.Fatalf("the quote = %+v", q)
		}
		if got.Reactions == nil || len(got.Reactions) != 0 {
			t.Fatalf("reactions = %+v", got.Reactions)
		}
	})

	t.Run("in a chat", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/chats/c1/messages", `{"text":"я","replyTo":"m2"}`, "Authorization", annAuth)
		got := decode[answerJSON](t, w)

		if w.Code != http.StatusCreated || h.chats.draft.ReplyToID != "m2" || got.ReplyTo == nil {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if q := got.ReplyTo; q.Author["nickname"] != "bob" || q.Text != "кто берёт палатку?" {
			t.Fatalf("the quote = %+v", q)
		}
		if len(got.Reactions) != 1 || got.Reactions[0].UserID != "u2" || got.Reactions[0].Reaction != "fire" {
			t.Fatalf("reactions = %+v", got.Reactions)
		}
	})

	t.Run("to one whose page is gone", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/chats/c1/messages", `{"text":"я","replyTo":"gone"}`, "Authorization", annAuth)
		got := decode[answerJSON](t, w)

		if q := got.ReplyTo; w.Code != http.StatusCreated || q == nil || q.ID != "gone" || q.Author != nil || q.Text != "" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	})
}

func TestReact(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	if w := h.do("PUT", "/api/v1/messages/m2/reaction", `{"reaction":"heart"}`, "Authorization", annAuth); w.Code != http.StatusNoContent || h.messages.reacted != "m2/heart" {
		t.Fatalf("put: %d %s, reacted %q", w.Code, w.Body, h.messages.reacted)
	}
	if w := h.do("DELETE", "/api/v1/messages/m2/reaction", "", "Authorization", annAuth); w.Code != http.StatusNoContent || h.messages.reacted != "m2/" {
		t.Fatalf("delete: %d, reacted %q", w.Code, h.messages.reacted)
	}
	if w := h.do("PUT", "/api/v1/messages/m2/reaction", `{}`, "Authorization", annAuth); w.Code != http.StatusBadRequest {
		t.Fatalf("no reaction: %d", w.Code)
	}

	if w := h.do("PUT", "/api/v1/chat-messages/m2/reaction", `{"reaction":"fire"}`, "Authorization", annAuth); w.Code != http.StatusNoContent {
		t.Fatalf("put in a chat: %d %s", w.Code, w.Body)
	}
	if w := h.do("DELETE", "/api/v1/chat-messages/m2/reaction", "", "Authorization", annAuth); w.Code != http.StatusNoContent {
		t.Fatalf("delete in a chat: %d", w.Code)
	}
	if want := []string{"react m2 fire", "react m2 "}; !slices.Equal(h.chats.calls, want) {
		t.Fatalf("calls = %q, want %q", h.chats.calls, want)
	}

	if w := h.do("PUT", "/api/v1/messages/m2/reaction", `{"reaction":"heart"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}
