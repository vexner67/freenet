package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeGifts knows the gifts g2 (anonymous, untold) and g1 (of u2, bob).
type fakeGifts struct {
	token, userID, cursor, deleted string
	priced                         string
	price                          int64
	limit                          int
	sent                           model.GiftDraft
	err                            error
	fakeMarket
}

var giftTime = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)

func (f *fakeGifts) Catalog(context.Context) (model.GiftCatalog, error) {
	return model.GiftCatalog{
		Collections: []model.GiftCollection{
			{ID: "flowers", Gifts: []string{"rose", "tulip"}, Prices: []int64{4, 0}, Supplies: []int{0, 10}, Sold: []int{3, 10}},
			{ID: "sweets", Gifts: []string{"cake"}, Price: 2},
		},
		Models:       []model.GiftTrait{{Name: "classic", Weight: 1000}},
		UpgradePrice: 50,
	}, f.err
}

func (f *fakeGifts) SetPrice(_ context.Context, token, gift string, price int64) error {
	f.token, f.priced, f.price = token, gift, price
	return f.err
}

func (f *fakeGifts) Send(_ context.Context, token, userID string, d model.GiftDraft) (model.Gift, error) {
	f.token, f.userID, f.sent = token, userID, d
	sender := model.PublicProfile{ID: "u1", Profile: model.Profile{Name: "Ann", Nickname: "ann"}}
	return model.Gift{ID: "g9", Gift: d.Gift, Caption: d.Caption, Anonymous: d.Anonymous, SentAt: giftTime, Sender: &sender}, f.err
}

func (f *fakeGifts) List(_ context.Context, token, userID, cursor string, limit int) (model.GiftPage, error) {
	f.token, f.userID, f.cursor, f.limit = token, userID, cursor, limit
	bob := model.PublicProfile{ID: "u2", AvatarID: "av2", Profile: model.Profile{Name: "Bob", Nickname: "bob"}, Online: true}
	return model.GiftPage{
		Gifts: []model.Gift{
			{ID: "g2", Gift: "cake", SentAt: giftTime},
			{ID: "g1", Gift: "rose", Caption: "Привет", SentAt: giftTime.Add(-time.Hour), Sender: &bob},
		},
		NextCursor: "g1", Total: 5,
	}, f.err
}

func (f *fakeGifts) Delete(_ context.Context, token, giftID string) error {
	f.token, f.deleted = token, giftID
	return f.err
}

type giftBody struct {
	ID        string         `json:"id"`
	Gift      string         `json:"gift"`
	Caption   string         `json:"caption"`
	Anonymous bool           `json:"anonymous"`
	SentAt    string         `json:"sentAt"`
	Sender    map[string]any `json:"sender"`
}

func TestGiftCatalog(t *testing.T) {
	t.Parallel()

	// Public.
	w := newHarness(rest.Config{}).do("GET", "/api/v1/gifts", "")
	got := decode[struct {
		Collections []struct {
			ID     string   `json:"id"`
			Gifts  []string `json:"gifts"`
			Price  int64    `json:"price"`
			Prices []int64  `json:"prices"`
		} `json:"collections"`
	}](t, w)

	if w.Code != http.StatusOK || len(got.Collections) != 2 || got.Collections[0].ID != "flowers" || len(got.Collections[0].Gifts) != 2 ||
		got.Collections[1].Gifts[0] != "cake" || got.Collections[0].Price != 0 || got.Collections[1].Price != 2 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	// A service that does not know the prices of gifts gives none: an
	// empty list, not null.
	if p := got.Collections[0].Prices; len(p) != 2 || p[0] != 4 || got.Collections[1].Prices == nil || len(got.Collections[1].Prices) != 0 {
		t.Fatalf("prices: %+v", got.Collections)
	}
}

func TestSetGiftPrice(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/gifts/rose/price", `{"price":3}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent ||
		h.gifts.priced != "rose" || h.gifts.price != 3 || h.gifts.token != "tok" {
		t.Fatalf("%d, priced %q at %d", w.Code, h.gifts.priced, h.gifts.price)
	}

	h = newHarness(rest.Config{})
	h.gifts.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("PUT", "/api/v1/gifts/rose/price", `{"price":3}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("not an administrator: %d", w.Code)
	}

	if w := newHarness(rest.Config{}).do("PUT", "/api/v1/gifts/rose/price", `{"price":3}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestListGifts(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/ann/gifts?cursor=g5&limit=2", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Gifts      []giftBody `json:"gifts"`
		NextCursor string     `json:"nextCursor"`
		Total      int        `json:"total"`
	}](t, w)

	if w.Code != http.StatusOK || got.NextCursor != "g1" || got.Total != 5 || len(got.Gifts) != 2 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.gifts.token != "tok" || h.gifts.userID != "u1" || h.gifts.cursor != "g5" || h.gifts.limit != 2 || h.users.accessAsked != "u1" {
		t.Fatalf("asked %+v, access of %q", h.gifts, h.users.accessAsked)
	}

	// An anonymous gift tells neither who nor what.
	if g := got.Gifts[0]; g.ID != "g2" || g.Gift != "cake" || g.Sender != nil || g.Caption != "" || g.SentAt != "2026-10-12T09:30:00Z" {
		t.Fatalf("the anonymous gift = %+v", g)
	}
	if g := got.Gifts[1]; g.Sender["nickname"] != "bob" || g.Sender["online"] != true || g.Caption != "Привет" || g.Sender["avatar"] == nil {
		t.Fatalf("the signed gift = %+v", g)
	}

	// No token is needed.
	h = newHarness(rest.Config{})
	if w = h.do("GET", "/api/v1/users/ann/gifts", ""); w.Code != http.StatusOK || h.gifts.token != "" {
		t.Fatalf("anonymous: %d", w.Code)
	}

	// "Кому видны мои подарки".
	h = newHarness(rest.Config{})
	h.users.hiddenGifts = true
	w = h.do("GET", "/api/v1/users/ann/gifts", "")
	if w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != "this user's gifts are hidden" || h.gifts.userID != "" {
		t.Fatalf("hidden gifts: %d %s", w.Code, w.Body)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/users/cat/gifts", ""); w.Code != http.StatusForbidden {
		t.Fatalf("a deleted page: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/users/nobody/gifts", ""); w.Code != http.StatusNotFound {
		t.Fatalf("nobody: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("GET", "/api/v1/users/ann/gifts?limit=x", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("a broken limit: %d", w.Code)
	}
}

func TestSendGift(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w := h.do("POST", "/api/v1/users/bob/gifts", `{"gift":"rose","caption":"С праздником!","anonymous":true}`, "Authorization", "Bearer tok")
	got := decode[giftBody](t, w)
	if w.Code != http.StatusCreated || got.ID != "g9" || got.Gift != "rose" || !got.Anonymous || got.Sender["nickname"] != "ann" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if want := (model.GiftDraft{Gift: "rose", Caption: "С праздником!", Anonymous: true}); h.gifts.sent != want || h.gifts.userID != "u2" || h.gifts.token != "tok" {
		t.Fatalf("sent %+v to %q", h.gifts.sent, h.gifts.userID)
	}

	// A gift is signed with a name: the profile has to be filled in.
	h = newHarness(rest.Config{})
	w = h.do("POST", "/api/v1/users/bob/gifts", `{"gift":"rose"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusBadRequest || decode[errorBody](t, w).Error.Message != "fill in your profile before sending gifts" || h.gifts.userID != "" {
		t.Fatalf("without a profile: %d %s", w.Code, w.Body)
	}

	// What the user service refuses comes with its words.
	h = newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	h.gifts.err = failure.New(failure.PermissionDenied, "you cannot send a gift to this user")
	w = h.do("POST", "/api/v1/users/bob/gifts", `{"gift":"rose"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != "you cannot send a gift to this user" {
		t.Fatalf("refused: %d %s", w.Code, w.Body)
	}

	h = newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	if w = h.do("POST", "/api/v1/users/nobody/gifts", `{"gift":"rose"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("to nobody: %d", w.Code)
	}
	// A gift the sender cannot pay for.
	h.gifts.err = failure.New(failure.Conflict, "not enough votes")
	w = h.do("POST", "/api/v1/users/bob/gifts", `{"gift":"cake"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusConflict || decode[errorBody](t, w).Error.Message != "not enough votes" {
		t.Fatalf("without votes: %d %s", w.Code, w.Body)
	}
	h.gifts.err = nil

	if w = h.do("POST", "/api/v1/users/bob/gifts", `{"gift":"rose","price":1}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown field: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/users/bob/gifts", `{"gift":"rose"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestDeleteGift(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("DELETE", "/api/v1/gifts/g1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.gifts.deleted != "g1" || h.gifts.token != "tok" {
		t.Fatalf("%d, deleted %q", w.Code, h.gifts.deleted)
	}

	h = newHarness(rest.Config{})
	h.gifts.err = failure.New(failure.NotFound, "gift not found")
	if w := h.do("DELETE", "/api/v1/gifts/nope", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("no such gift: %d", w.Code)
	}

	if w := newHarness(rest.Config{}).do("DELETE", "/api/v1/gifts/g1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}
