package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type paidPost struct {
	Photos          []map[string]string `json:"photos"`
	Price           int64               `json:"price"`
	Locked          bool                `json:"locked"`
	LockedPhotos    int                 `json:"lockedPhotos"`
	LockedDocuments int                 `json:"lockedDocuments"`
}

func TestPaidPost(t *testing.T) {
	t.Parallel()

	t.Run("locked to whoever has not paid", func(t *testing.T) {
		t.Parallel()

		for _, headers := range [][]string{nil, {"Authorization", "Bearer tok"}} {
			w := newHarness(rest.Config{}).do("GET", "/api/v1/posts/p9", "", headers...)
			got := decode[paidPost](t, w)
			if w.Code != http.StatusOK || !got.Locked || len(got.Photos) != 0 || got.LockedPhotos != 2 || got.LockedDocuments != 1 || got.Price != 15 {
				t.Fatalf("%v: %d %+v", headers, w.Code, got)
			}
		}
	})

	t.Run("open to its author", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("GET", "/api/v1/posts/p9", "", "Authorization", "Bearer ann")
		if got := decode[paidPost](t, w); w.Code != http.StatusOK || got.Locked || len(got.Photos) != 2 || got.Price != 15 {
			t.Fatalf("%d %+v", w.Code, got)
		}
	})

	t.Run("bought", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/posts/p9/purchase", "", "Authorization", "Bearer tok")
		got := decode[struct {
			Balance int64    `json:"balance"`
			Post    paidPost `json:"post"`
		}](t, w)
		if w.Code != http.StatusOK || got.Balance != 85 || got.Post.Locked || len(got.Post.Photos) != 2 {
			t.Fatalf("%d %+v", w.Code, got)
		}
		if len(h.users.purchases) != 1 || h.users.purchases[0] != "p9:u1:15" {
			t.Fatalf("purchases = %v", h.users.purchases)
		}

		// Paid, it stays open.
		if got := decode[paidPost](t, h.do("GET", "/api/v1/posts/p9", "", "Authorization", "Bearer tok")); got.Locked {
			t.Fatalf("after the purchase: %+v", got)
		}
	})

	t.Run("not for sale", func(t *testing.T) {
		t.Parallel()

		for _, path := range []string{"/api/v1/posts/p1/purchase", "/api/v1/posts/p9/purchase"} {
			token := "Bearer tok"
			if path == "/api/v1/posts/p9/purchase" {
				token = "Bearer ann"
			}
			h := newHarness(rest.Config{})
			if w := h.do("POST", path, "", "Authorization", token); w.Code != http.StatusConflict || len(h.users.purchases) != 0 {
				t.Fatalf("%s: %d", path, w.Code)
			}
		}
	})
}

func TestPaidMessages(t *testing.T) {
	t.Parallel()

	t.Run("paid at the price", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		h.users.messagePrice = 20

		page := decode[struct {
			CanWrite     bool  `json:"canWrite"`
			MessagePrice int64 `json:"messagePrice"`
		}](t, h.do("GET", "/api/v1/dialogs/bob/messages", "", "Authorization", "Bearer tok"))
		if !page.CanWrite || page.MessagePrice != 20 {
			t.Fatalf("page = %+v", page)
		}

		if w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"cHJpdmV0"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict || h.messages.sent != "" {
			t.Fatalf("without the price: %d", w.Code)
		}

		if w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"cHJpdmV0","price":20}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || h.messages.sent != "privet" {
			t.Fatalf("paid: %d %s", w.Code, w.Body)
		}
		if len(h.users.paid) != 1 || h.users.paid[0] != 20 || len(h.users.refunded) != 0 {
			t.Fatalf("paid %v, refunded %v", h.users.paid, h.users.refunded)
		}
	})

	t.Run("given back when not sent", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		h.users.messagePrice = 20

		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"bm9wZQ==","price":20}`, "Authorization", "Bearer tok")
		if w.Code != http.StatusBadRequest || len(h.users.refunded) != 1 || h.users.refunded[0] != "payment" {
			t.Fatalf("%d, refunded %v", w.Code, h.users.refunded)
		}
	})

	t.Run("only Freenet Prime", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		h.users.primeOnly = true

		page := decode[struct {
			CanWrite  bool `json:"canWrite"`
			PrimeOnly bool `json:"primeOnly"`
		}](t, h.do("GET", "/api/v1/dialogs/bob/messages", "", "Authorization", "Bearer tok"))
		if page.CanWrite || !page.PrimeOnly {
			t.Fatalf("page = %+v", page)
		}

		w := h.do("POST", "/api/v1/dialogs/bob/messages", `{"sealed":"cHJpdmV0"}`, "Authorization", "Bearer tok")
		if got := decode[errorBody](t, w); w.Code != http.StatusForbidden || got.Error.Message != "only people with freenet prime can write to this user" {
			t.Fatalf("%d %+v", w.Code, got)
		}
	})
}
