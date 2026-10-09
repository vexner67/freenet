package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type aboutResponse struct {
	Hometown      string  `json:"hometown"`
	HideBirthYear bool    `json:"hideBirthYear"`
	Birthday      *string `json:"birthday"`
	Contacts      struct {
		City     string `json:"city"`
		Telegram string `json:"telegram"`
	} `json:"contacts"`
	Interests struct {
		Films string `json:"films"`
	} `json:"interests"`
	Fields []struct {
		Name  string `json:"name"`
		Place string `json:"place"`
	} `json:"fields"`
	RegisteredAt string         `json:"registeredAt"`
	Partner      map[string]any `json:"partner"`
}

func TestEditTheAboutOfThePage(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	w := h.do("PUT", "/api/v1/me/profile", `{"name":"Ann","nickname":"ann","relationship":"engaged","partner":"bob","hometown":"Тверь","hideBirthYear":true}`, auth...)
	got := decode[aboutResponse](t, w)
	if w.Code != http.StatusOK || h.users.partner != "bob" || got.Hometown != "Тверь" || !got.HideBirthYear || got.Partner["nickname"] != "bob" {
		t.Fatalf("profile: %d %+v, partner %q", w.Code, got, h.users.partner)
	}

	w = h.do("PUT", "/api/v1/me/contacts", `{"telegram":"ann_tg","city":"Москва"}`, auth...)
	if got = decode[aboutResponse](t, w); w.Code != http.StatusOK || got.Contacts.Telegram != "ann_tg" || h.users.about.Contacts.City != "Москва" {
		t.Fatalf("contacts: %d %+v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/me/interests", `{"films":"Брат"}`, auth...)
	if got = decode[aboutResponse](t, w); w.Code != http.StatusOK || got.Interests.Films != "Брат" {
		t.Fatalf("interests: %d %+v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/me/fields", `{"fields":[{"name":"Цвет","text":"синий","place":"contacts"}]}`, auth...)
	if got = decode[aboutResponse](t, w); w.Code != http.StatusOK || len(got.Fields) != 1 || got.Fields[0].Place != "contacts" {
		t.Fatalf("fields: %d %+v", w.Code, got)
	}
	if want := []model.ExtraField{{Name: "Цвет", Text: "синий", Place: "contacts"}}; len(h.users.about.ExtraFields) != 1 || h.users.about.ExtraFields[0] != want[0] {
		t.Fatalf("fields stored = %+v", h.users.about.ExtraFields)
	}

	if w = h.do("PUT", "/api/v1/me/contacts", `{"city":"Москва"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}

func TestTheAboutOfAPage(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/dina", "")
	got := decode[aboutResponse](t, w)

	// Without its year the birthday is "--MM-DD"; whom the relationship
	// names comes as an author.
	if w.Code != http.StatusOK || got.Birthday == nil || *got.Birthday != "--05-17" || got.HideBirthYear ||
		got.Partner["nickname"] != "bob" || got.Contacts.City != "Москва" || len(got.Fields) != 1 || got.RegisteredAt != "2026-10-01T09:00:00Z" {
		t.Fatalf("%d %+v", w.Code, got)
	}
}

func TestSearchUsersByFilters(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users?city=Москва&film=Брат", "")
	want := model.UserQuery{City: "Москва", Film: "Брат"}
	if w.Code != http.StatusOK || h.users.search != want {
		t.Fatalf("%d, asked %+v", w.Code, h.users.search)
	}

	if w = h.do("GET", "/api/v1/users", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("nothing to search: %d", w.Code)
	}
}
