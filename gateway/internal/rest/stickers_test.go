package rest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeStickers has smiles added for "tok" and cats bought but put away.
type fakeStickers struct {
	token, added, removed string
	err                   error
}

func (f *fakeStickers) Catalog(_ context.Context, token string) ([]model.StickerPack, error) {
	f.token = token
	packs := []model.StickerPack{
		{ID: "smiles", Stickers: []string{"smile_happy", "smile_sad"}},
		{ID: "cats", Price: 2, Stickers: []string{"cat_hi"}},
		{ID: "bears", Price: 4, Stickers: []string{"bear_hi"}},
	}
	if token == "tok" {
		packs[0].Bought, packs[0].Added = true, true
		packs[1].Bought = true
	}

	return packs, f.err
}

func (f *fakeStickers) Add(_ context.Context, token, packID string) error {
	f.token, f.added = token, packID
	return f.err
}

func (f *fakeStickers) Remove(_ context.Context, token, packID string) error {
	f.token, f.removed = token, packID
	return f.err
}

type stickerPackBody struct {
	ID       string   `json:"id"`
	Price    int64    `json:"price"`
	Stickers []string `json:"stickers"`
	Bought   bool     `json:"bought"`
	Added    bool     `json:"added"`
}

func TestStickerCatalog(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/stickers", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Packs []stickerPackBody `json:"packs"`
	}](t, w)

	if w.Code != http.StatusOK || len(got.Packs) != 3 || h.stickers.token != "tok" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if p := got.Packs[0]; p.ID != "smiles" || p.Price != 0 || !p.Bought || !p.Added || len(p.Stickers) != 2 {
		t.Fatalf("smiles = %+v", p)
	}
	if p := got.Packs[1]; p.ID != "cats" || p.Price != 2 || !p.Bought || p.Added {
		t.Fatalf("cats = %+v", p)
	}

	// Public.
	h = newHarness(rest.Config{})
	if w = h.do("GET", "/api/v1/stickers", ""); w.Code != http.StatusOK || h.stickers.token != "" {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestStickerPacks(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/me/stickers/cats", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.stickers.added != "cats" || h.stickers.token != "tok" {
		t.Fatalf("add: %d %+v", w.Code, h.stickers)
	}
	if w := h.do("DELETE", "/api/v1/me/stickers/cats", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.stickers.removed != "cats" {
		t.Fatalf("remove: %d %+v", w.Code, h.stickers)
	}

	// Too few votes.
	h = newHarness(rest.Config{})
	h.stickers.err = failure.New(failure.Conflict, "not enough votes")
	w := h.do("PUT", "/api/v1/me/stickers/bears", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusConflict || decode[errorBody](t, w).Error.Message != "not enough votes" {
		t.Fatalf("no votes: %d %s", w.Code, w.Body)
	}

	if w = newHarness(rest.Config{}).do("PUT", "/api/v1/me/stickers/cats", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestSendSticker(t *testing.T) {
	t.Parallel()

	// filled is a harness whose caller has filled in their profile.
	filled := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		return h
	}

	h := filled()
	w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sticker":"smile_sad"}`, "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusCreated || got["sticker"] != "smile_sad" {
		t.Fatalf("%d %v", w.Code, got)
	}

	// A pack put away, a pack never taken, no such sticker.
	for sticker, code := range map[string]int{"cat_hi": http.StatusForbidden, "bear_hi": http.StatusForbidden, "dog_hi": http.StatusBadRequest} {
		h = filled()
		if w = h.do("POST", "/api/v1/dialogs/bob/messages", `{"sticker":"`+sticker+`"}`, "Authorization", "Bearer tok"); w.Code != code {
			t.Fatalf("%s: %d %s", sticker, w.Code, w.Body)
		}
	}

	// In a chat too.
	h = filled()
	w = h.do("POST", "/api/v1/chats/c1/messages", `{"sticker":"smile_happy"}`, "Authorization", "Bearer tok")
	if got = decode[map[string]any](t, w); w.Code != http.StatusCreated || got["sticker"] != "smile_happy" || h.chats.draft.Sticker != "smile_happy" {
		t.Fatalf("chat: %d %v", w.Code, got)
	}
	h = filled()
	if w = h.do("POST", "/api/v1/chats/c1/messages", `{"sticker":"cat_hi"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("chat, a pack put away: %d", w.Code)
	}
}
