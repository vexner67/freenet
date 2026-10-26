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

var petTime = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

type fakePets struct {
	token    string
	nickname string
	board    string
	kind     string
	name     string
	shop     model.PetShop
	noPet    bool
	err      error
}

func (f *fakePets) pet() model.Pet {
	return model.Pet{Kind: "cat", Name: "Барсик", BornAt: petTime, Satiety: 80, Joy: 70, Level: 2, Rating: 1016, Attack: 12}
}

func (f *fakePets) fight() model.PetFight {
	return model.PetFight{
		ID: "f1", AttackerID: "u1", DefenderID: "u3", FoughtAt: petTime,
		Attacker: model.PetFighter{Kind: "cat", Name: "Барсик", Level: 2, Health: 60},
		Defender: model.PetFighter{Kind: "dog", Name: "Шарик", Level: 1, Health: 50},
		Hits:     []model.PetHit{{ByAttacker: true, Crit: true, Damage: 50, AttackerHealth: 60}},
		Won:      true, Rating: 16,
	}
}

func (f *fakePets) Visit(_ context.Context, token, nickname string) error {
	f.token, f.nickname = token, nickname
	return f.err
}

func (f *fakePets) Guests(_ context.Context, token, cursor string, _ int) (model.GuestPage, error) {
	f.token = token
	return model.GuestPage{
		Guests:     []model.Guest{{UserID: "u2", VisitedAt: petTime}, {UserID: "u3", VisitedAt: petTime}, {UserID: "u1", VisitedAt: petTime.Add(-time.Hour)}},
		SeenAt:     petTime.Add(-time.Minute),
		NextCursor: "next",
	}, f.err
}

func (f *fakePets) Top(_ context.Context, token, board string) ([]model.TopEntry, error) {
	f.token, f.board = token, board
	return []model.TopEntry{{UserID: "u3", Value: 900}, {UserID: "u2", Value: 500, PetKind: "dog", PetName: "Шарик"}, {UserID: "u1", Value: 100}}, f.err
}

func (f *fakePets) Get(_ context.Context, token string) (model.PetState, error) {
	f.token = token
	state := model.PetState{Shop: model.PetShop{Prices: map[string]int64{"cat": 100}, Meal: 5, Win: 3, Fights: 10}}
	if !f.noPet {
		p := f.pet()
		state.Pet = &p
	}
	return state, f.err
}

func (f *fakePets) UserPet(_ context.Context, token, nickname string) (model.Pet, string, error) {
	f.token, f.nickname = token, nickname
	return f.pet(), "u2", f.err
}

func (f *fakePets) Adopt(_ context.Context, token, kind, name string) (model.PetCare, error) {
	f.token, f.kind, f.name = token, kind, name
	return model.PetCare{Pet: f.pet(), Balance: 900}, f.err
}

func (f *fakePets) Feed(_ context.Context, token string) (model.PetCare, error) {
	f.token = token
	return model.PetCare{Pet: f.pet(), Balance: 895}, f.err
}

func (f *fakePets) Play(_ context.Context, token string) (model.PetCare, error) {
	f.token = token
	p := f.pet()
	p.PlayAt = petTime.Add(time.Hour)
	return model.PetCare{Pet: p}, f.err
}

func (f *fakePets) Rename(_ context.Context, token, name string) (model.PetCare, error) {
	f.token, f.name = token, name
	return model.PetCare{Pet: f.pet()}, f.err
}

func (f *fakePets) Release(_ context.Context, token string) error {
	f.token = token
	return f.err
}

func (f *fakePets) Fight(_ context.Context, token, nickname string) (model.PetFought, error) {
	f.token, f.nickname = token, nickname
	return model.PetFought{Fight: f.fight(), Pet: f.pet(), Balance: 903}, f.err
}

func (f *fakePets) Fights(_ context.Context, token string) ([]model.PetFight, error) {
	f.token = token
	return []model.PetFight{f.fight()}, f.err
}

func (f *fakePets) Settings(_ context.Context, token string) (model.PetShop, model.PetStats, error) {
	f.token = token
	return model.PetShop{Prices: map[string]int64{"cat": 100}, Meal: 5, Win: 3, Fights: 10}, model.PetStats{Pets: 2, Fights: 7, Spent: 155, Won: 21}, f.err
}

func (f *fakePets) Set(_ context.Context, token string, shop model.PetShop) error {
	f.token, f.shop = token, shop
	return f.err
}

func TestGuestsAndTop(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	if w := h.do("POST", "/api/v1/users/bob/visits", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("a visit of a guest: %d", w.Code)
	}
	if w := h.do("POST", "/api/v1/users/bob/visits", "", auth...); w.Code != http.StatusNoContent || h.pets.nickname != "bob" {
		t.Fatalf("visit: %d %q", w.Code, h.pets.nickname)
	}

	// The deleted page of u3 is left out; u1 came before the last look.
	w := h.do("GET", "/api/v1/me/guests", "", auth...)
	guests := decode[struct {
		Guests []struct {
			User      map[string]any `json:"user"`
			VisitedAt string         `json:"visitedAt"`
			New       bool           `json:"new"`
		} `json:"guests"`
		SeenAt     string `json:"seenAt"`
		NextCursor string `json:"nextCursor"`
	}](t, w)
	if w.Code != http.StatusOK || len(guests.Guests) != 2 || guests.Guests[0].User["nickname"] != "bob" || !guests.Guests[0].New ||
		guests.Guests[1].New || guests.NextCursor != "next" || guests.SeenAt == "" {
		t.Fatalf("guests: %d %+v", w.Code, guests)
	}

	w = h.do("GET", "/api/v1/top?board=pets", "")
	top := decode[struct {
		Entries []struct {
			Place int            `json:"place"`
			User  map[string]any `json:"user"`
			Value int64          `json:"value"`
			Pet   map[string]any `json:"pet"`
		} `json:"entries"`
	}](t, w)
	if w.Code != http.StatusOK || h.pets.board != "pets" || len(top.Entries) != 2 || top.Entries[0].Place != 1 ||
		top.Entries[0].User["nickname"] != "bob" || top.Entries[0].Pet["name"] != "Шарик" || top.Entries[1].Pet != nil || top.Entries[1].Place != 2 {
		t.Fatalf("top: %d %+v", w.Code, top)
	}

	h.pets.err = failure.New(failure.InvalidArgument, "unknown board")
	if w = h.do("GET", "/api/v1/top?board=x", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad board: %d", w.Code)
	}
}

func TestPet(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	h.pets.noPet = true
	w := h.do("GET", "/api/v1/me/pet", "", auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["pet"] != nil || got["shop"].(map[string]any)["meal"] != 5.0 {
		t.Fatalf("no pet: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/pet", `{"kind":"cat","name":"Барсик"}`, auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusCreated || h.pets.kind != "cat" || h.pets.name != "Барсик" ||
		got["balance"] != 900.0 || got["pet"].(map[string]any)["rating"] != 1016.0 || got["pet"].(map[string]any)["playAt"] != nil {
		t.Fatalf("adopt: %d %v", w.Code, got)
	}

	if w = h.do("POST", "/api/v1/me/pet/play", "", auth...); decode[map[string]any](t, w)["pet"].(map[string]any)["playAt"] != "2026-10-26T13:00:00Z" {
		t.Fatalf("play: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/me/pet/feed", "", auth...); w.Code != http.StatusOK {
		t.Fatalf("feed: %d", w.Code)
	}
	if w = h.do("PATCH", "/api/v1/me/pet", `{"name":"Кекс"}`, auth...); w.Code != http.StatusOK || h.pets.name != "Кекс" {
		t.Fatalf("rename: %d", w.Code)
	}

	// The defender's page is deleted: the fight shows no owner of it.
	w = h.do("POST", "/api/v1/me/pet/fights", `{"nickname":"cat"}`, auth...)
	fought := decode[struct {
		Fight struct {
			Attacker map[string]any   `json:"attacker"`
			Defender map[string]any   `json:"defender"`
			Hits     []map[string]any `json:"hits"`
			Won      bool             `json:"won"`
		} `json:"fight"`
		Balance int64 `json:"balance"`
	}](t, w)
	if w.Code != http.StatusOK || h.pets.nickname != "cat" || fought.Balance != 903 || !fought.Fight.Won ||
		fought.Fight.Attacker["user"].(map[string]any)["nickname"] != "ann" || fought.Fight.Defender["user"] != nil ||
		fought.Fight.Hits[0]["crit"] != true {
		t.Fatalf("fight: %d %+v", w.Code, fought)
	}

	if w = h.do("GET", "/api/v1/me/pet/fights", "", auth...); len(decode[map[string]any](t, w)["fights"].([]any)) != 1 {
		t.Fatalf("fights: %d", w.Code)
	}
	if w = h.do("GET", "/api/v1/users/bob/pet", ""); w.Code != http.StatusOK || decode[map[string]any](t, w)["name"] != "Барсик" {
		t.Fatalf("a pet of a page: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/pet/settings", "", auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["spent"] != 155.0 || got["shop"].(map[string]any)["fights"] != 10.0 {
		t.Fatalf("settings: %d %v", w.Code, got)
	}
	w = h.do("PUT", "/api/v1/pet/settings", `{"prices":{"cat":10,"dog":20,"hamster":5,"dragon":50},"meal":1,"win":2,"fights":3}`, auth...)
	if w.Code != http.StatusNoContent || h.pets.shop.Prices["dragon"] != 50 || h.pets.shop.Fights != 3 {
		t.Fatalf("set: %d %+v", w.Code, h.pets.shop)
	}

	if w = h.do("DELETE", "/api/v1/me/pet", "", auth...); w.Code != http.StatusNoContent {
		t.Fatalf("release: %d", w.Code)
	}

	h.pets.err = failure.New(failure.Conflict, "the pet is hungry")
	if w = h.do("POST", "/api/v1/me/pet/fights", `{}`, auth...); w.Code != http.StatusConflict {
		t.Fatalf("a hungry pet: %d", w.Code)
	}
}
