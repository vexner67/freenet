package rest_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// stickerPhotoID is the photo of the sticker of u1 the fakes know.
const (
	stickerPhotoID = "0198c0de-0000-7000-8000-000000000001"
	photoSticker   = "0198c0de000070008000000000000001"
)

// fakeStickers has smiles added for "tok" and cats bought but put away.
type fakeStickers struct {
	token, added, removed string
	// draft is what the last Create or Edit was told, of pack.
	draft model.StickerPackDraft
	pack  string
	// gone are the photos Edit and Delete say a pack no longer has.
	gone []string
	err  error
}

func (f *fakeStickers) Mine(_ context.Context, token string) ([]model.StickerPack, error) {
	f.token = token
	return []model.StickerPack{{ID: "p1", Price: 3, Stickers: []string{photoSticker}, Bought: true, Added: true, AuthorID: "u1", Name: "Котики", Holders: 2}}, f.err
}

func (f *fakeStickers) Create(_ context.Context, token string, d model.StickerPackDraft) (model.StickerPack, error) {
	f.token, f.draft = token, d
	return model.StickerPack{ID: "p1", Price: d.Price, Stickers: d.Added, AuthorID: "u1", Name: d.Name, Published: d.Published}, f.err
}

func (f *fakeStickers) Edit(_ context.Context, token, packID string, d model.StickerPackDraft) (model.StickerPack, []string, error) {
	f.token, f.pack, f.draft = token, packID, d
	return model.StickerPack{ID: packID, AuthorID: "u1", Name: d.Name}, f.gone, f.err
}

func (f *fakeStickers) Delete(_ context.Context, token, packID string) ([]string, error) {
	f.token, f.pack = token, packID
	return f.gone, f.err
}

func (f *fakeStickers) Author(_ context.Context, sticker string) (string, error) {
	if sticker == photoSticker {
		return "u1", nil
	}

	return "", nil
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

func TestStickerStudio(t *testing.T) {
	t.Parallel()

	// filled is a harness whose caller has filled in their profile.
	filled := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		return h
	}

	h := filled()
	w := h.do("POST", "/api/v1/me/sticker-packs", `{"name":"Котики","price":3,"published":true,"photos":["ph1","ph2"]}`, "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	author, _ := got["author"].(map[string]any)
	if w.Code != http.StatusCreated || got["name"] != "Котики" || author["nickname"] != "ann" || h.stickers.draft.Price != 3 ||
		!slices.Equal(h.media.claimed, []string{"ph1,ph2"}) || !slices.Equal(h.media.published, []string{"u1/ph1,ph2"}) {
		t.Fatalf("create: %d %v, media %+v", w.Code, got, h.media)
	}

	// A photo not the caller's, a profile not filled in, too many photos.
	h = filled()
	if w = h.do("POST", "/api/v1/me/sticker-packs", `{"name":"x","photos":["ph7"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.stickers.draft.Name != "" {
		t.Fatalf("a photo not mine: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/sticker-packs", `{"name":"x","photos":["ph1"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest ||
		decode[errorBody](t, w).Error.Message != "fill in your profile before making stickers" {
		t.Fatalf("no profile: %d %s", w.Code, w.Body)
	}
	if w = filled().do("POST", "/api/v1/me/sticker-packs", `{"name":"x","photos":["1","2","3","4","5","6","7","8","9","10","11"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("too many photos: %d", w.Code)
	}

	// Refused, the photos kept for it go.
	h = filled()
	h.stickers.err = failure.New(failure.InvalidArgument, "sticker pack name is empty")
	if w = h.do("POST", "/api/v1/me/sticker-packs", `{"name":"","photos":["ph1"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest ||
		!slices.Equal(h.media.unpublished, []string{"u1/ph1"}) {
		t.Fatalf("refused: %d, media %+v", w.Code, h.media)
	}

	// An edit keeps what it adds and deletes what went.
	h = filled()
	h.stickers.gone = []string{"old"}
	w = h.do("PATCH", "/api/v1/sticker-packs/p1", `{"name":"Кошки","addPhotos":["ph2"],"removePhotos":["old"]}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.stickers.pack != "p1" || !slices.Equal(h.stickers.draft.Removed, []string{"old"}) ||
		!slices.Equal(h.media.published, []string{"u1/ph2"}) || !slices.Equal(h.media.unpublished, []string{"u1/old"}) {
		t.Fatalf("edit: %d %s, media %+v", w.Code, w.Body, h.media)
	}

	h = filled()
	h.stickers.gone = []string{"s1", "s2"}
	if w = h.do("DELETE", "/api/v1/sticker-packs/p1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || !slices.Equal(h.media.unpublished, []string{"u1/s1,s2"}) {
		t.Fatalf("delete: %d, media %+v", w.Code, h.media)
	}

	h = filled()
	h.stickers.err = failure.New(failure.Conflict, "the sticker pack is taken by others")
	if w = h.do("DELETE", "/api/v1/sticker-packs/p1", "", "Authorization", "Bearer tok"); w.Code != http.StatusConflict || len(h.media.unpublished) != 0 {
		t.Fatalf("delete a taken pack: %d, media %+v", w.Code, h.media)
	}

	w = filled().do("GET", "/api/v1/me/sticker-packs", "", "Authorization", "Bearer tok")
	mine := decode[struct {
		Packs []map[string]any `json:"packs"`
	}](t, w)
	if w.Code != http.StatusOK || len(mine.Packs) != 1 || mine.Packs[0]["holders"] != float64(2) {
		t.Fatalf("mine: %d %v", w.Code, mine)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/me/sticker-packs", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestServeSticker(t *testing.T) {
	t.Parallel()

	w := newHarness(rest.Config{}).do("GET", "/media/stickers/"+photoSticker, "")
	if w.Code != http.StatusOK || w.Body.String() != "small-photo" || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("%d %q", w.Code, w.Body)
	}

	for _, sticker := range []string{"cat_hi", "0198c0de000070008000000000000002", "0198C0DE000070008000000000000001"} {
		if w = newHarness(rest.Config{}).do("GET", "/media/stickers/"+sticker, ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", sticker, w.Code)
		}
	}
}
