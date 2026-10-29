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
	petID    string
	nickname string
	board    string
	kind     string
	name     string
	item     string
	wear     bool
	stance   string
	move     string
	shards   bool
	shop     model.PetShop
	noPet    bool
	arena    model.PetArenaAction
	err      error
}

func (f *fakePets) pet() model.Pet {
	return model.Pet{ID: "p1", Kind: "cat", Name: "Барсик", BornAt: petTime, Satiety: 80, Joy: 70, Level: 2, Rating: 1016, Attack: 12}
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
	state := model.PetState{
		Shop:   model.PetShop{Prices: map[string]int64{"cat": 100}, Cases: map[string]int64{"gold": 1200}, Meal: 5, Win: 3, Fights: 10},
		Kinds:  []model.PetKind{{Kind: "phoenix", Rarity: "legendary", Stats: model.PetPower{Attack: 12}, Crit: 14}},
		Cases:  []model.PetCase{{ID: "gold", Odds: []int{0, 550, 370, 80}, Shards: 60}},
		Keeper: model.PetKeeper{Shards: 5, PityLimit: 40, DropKind: "owl", DropCase: "gold"},
		Items:  []model.PetItem{{ID: "crown", Slot: "head", Price: 1500, Bonus: model.PetPower{Attack: 3, Defense: 3}}},
	}
	if !f.noPet {
		state.Pets = []model.Pet{f.pet()}
		state.Battles = []model.PetBattle{f.battle()}
	}
	return state, f.err
}

func (f *fakePets) UserPets(_ context.Context, token, nickname string) ([]model.Pet, string, error) {
	f.token, f.nickname = token, nickname
	return []model.Pet{f.pet()}, "u2", f.err
}

func (f *fakePets) Adopt(_ context.Context, token, kind, name string) (model.PetCare, error) {
	f.token, f.kind, f.name = token, kind, name
	return model.PetCare{Pet: f.pet(), Balance: 900}, f.err
}

func (f *fakePets) Feed(_ context.Context, token, petID string) (model.PetCare, error) {
	f.token, f.petID = token, petID
	return model.PetCare{Pet: f.pet(), Balance: 895}, f.err
}

func (f *fakePets) Play(_ context.Context, token, petID string) (model.PetCare, error) {
	f.token, f.petID = token, petID
	p := f.pet()
	p.PlayAt = petTime.Add(time.Hour)
	return model.PetCare{Pet: p}, f.err
}

func (f *fakePets) Rename(_ context.Context, token, petID, name string) (model.PetCare, error) {
	f.token, f.petID, f.name = token, petID, name
	return model.PetCare{Pet: f.pet()}, f.err
}

func (f *fakePets) Release(_ context.Context, token, petID string) error {
	f.token, f.petID = token, petID
	return f.err
}

func (f *fakePets) Fight(_ context.Context, token, petID, nickname, stance string) (model.PetFought, error) {
	f.token, f.petID, f.nickname, f.stance = token, petID, nickname, stance
	return model.PetFought{Fight: f.fight(), Pet: f.pet(), Balance: 903}, f.err
}

func (f *fakePets) SetStance(_ context.Context, token, petID, stance string) (model.PetCare, error) {
	f.token, f.petID, f.stance = token, petID, stance
	pet := f.pet()
	pet.Stance = stance
	return model.PetCare{Pet: pet}, f.err
}

func (f *fakePets) battle() model.PetBattle {
	return model.PetBattle{
		PetID: "p1", DefenderID: "u2", Attacker: model.PetFighter{Kind: "cat", Stance: "attack"}, Defender: model.PetFighter{Kind: "dog", Stance: "defense"},
		Hits: []model.PetHit{{ByAttacker: true, Move: "guard", Heal: 2, AttackerHealth: 40}}, Round: 1, Rounds: 8, AttackerHealth: 40, DefenderHealth: 46,
		Ratio: 100, Win: 16, Loss: 16,
	}
}

func (f *fakePets) StartBattle(_ context.Context, token, petID, nickname, stance string) (model.PetBattleTurn, error) {
	f.token, f.petID, f.nickname, f.stance = token, petID, nickname, stance
	return model.PetBattleTurn{Battle: f.battle(), Pet: f.pet()}, f.err
}

func (f *fakePets) MoveBattle(_ context.Context, token, petID, move string) (model.PetBattleTurn, error) {
	f.token, f.petID, f.move = token, petID, move
	fight := f.fight()
	return model.PetBattleTurn{Battle: f.battle(), Pet: f.pet(), Fight: &fight, Balance: 905}, f.err
}

func (f *fakePets) Fights(_ context.Context, token string) ([]model.PetFight, error) {
	f.token = token
	return []model.PetFight{f.fight()}, f.err
}

func (f *fakePets) OpenCase(_ context.Context, token, caseID string, withShards bool) (model.PetDrop, error) {
	f.token, f.kind, f.shards = token, caseID, withShards
	return model.PetDrop{Keeper: model.PetKeeper{Pity: 1, PityLimit: 40, DropKind: "fox", DropCase: caseID}, Balance: 300}, f.err
}

func (f *fakePets) TakeDrop(_ context.Context, token, name string) (model.PetCare, error) {
	f.token, f.name = token, name
	return model.PetCare{Pet: f.pet()}, f.err
}

func (f *fakePets) LeaveDrop(_ context.Context, token string) (model.PetKeeper, error) {
	f.token = token
	return model.PetKeeper{Shards: 3, PityLimit: 40}, f.err
}

func (f *fakePets) BuyItem(_ context.Context, token, petID, item string) (model.PetCare, error) {
	f.token, f.petID, f.item = token, petID, item
	pet := f.pet()
	pet.Wardrobe, pet.Outfit, pet.Clothes = []string{item}, []string{item}, model.PetPower{Health: 12}
	return model.PetCare{Pet: pet, Balance: 150}, f.err
}

func (f *fakePets) Dress(_ context.Context, token, petID, item string, wear bool) (model.PetCare, error) {
	f.token, f.petID, f.item, f.wear = token, petID, item, wear
	return model.PetCare{Pet: f.pet()}, f.err
}

func (f *fakePets) Settings(_ context.Context, token string) (model.PetShop, model.PetStats, error) {
	f.token = token
	return model.PetShop{Prices: map[string]int64{"cat": 100}, Meal: 5, Win: 3, Fights: 10}, model.PetStats{Pets: 2, Fights: 7, Spent: 155, Won: 21}, f.err
}

func (f *fakePets) Set(_ context.Context, token string, shop model.PetShop) error {
	f.token, f.shop = token, shop
	return f.err
}

func (f *fakePets) arenaOf() model.PetArena {
	return model.PetArena{
		Boss: model.PetBoss{Kind: "dragon", Health: 100, MaxHealth: 200, KilledBy: "", Hits: []model.PetBossHit{{UserID: "u1", Damage: 100, Hits: 2}}},
		Tournament: model.PetTournament{
			Held: true, Entrants: []model.PetEntrant{{OwnerID: "u1", PetID: "p1"}, {OwnerID: "u3", PetID: "p3"}},
			Rounds: [][]model.PetMatch{{{A: 0, B: 1, Winner: 0, Bout: new(f.fight())}}}, Places: []int{0, 1},
		},
		Seasons: []model.PetSeason{{Season: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Places: []model.PetSeasonPlace{{OwnerID: "u1", Prize: 2000}}}},
		Quests:  []model.PetQuest{{Kind: "win", Goal: 3, Progress: 1}},
		Market:  []model.PetListing{{OwnerID: "u1", Pet: f.pet()}},
	}
}

func (f *fakePets) Arena(_ context.Context, token string) (model.PetArena, error) {
	f.token = token
	return f.arenaOf(), f.err
}

func (f *fakePets) RunArena(_ context.Context, token string, a model.PetArenaAction) (model.PetArenaDone, error) {
	f.token, f.arena = token, a
	bout := f.fight()

	return model.PetArenaDone{Arena: f.arenaOf(), Bout: &bout, Balance: 77}, f.err
}

func TestPetArena(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	if w := h.do("GET", "/api/v1/me/pets/arena", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("the arena of a guest: %d", w.Code)
	}

	// The owner of p3 is deleted: the tournament shows no owner of it.
	w := h.do("GET", "/api/v1/me/pets/arena", "", auth...)
	got := decode[map[string]any](t, w)
	tournament := got["tournament"].(map[string]any)
	entrants := tournament["entrants"].([]any)
	if w.Code != http.StatusOK || got["boss"].(map[string]any)["killedBy"] != nil ||
		got["boss"].(map[string]any)["hits"].([]any)[0].(map[string]any)["user"].(map[string]any)["nickname"] != "ann" ||
		entrants[1].(map[string]any)["user"] != nil || len(tournament["rounds"].([]any)) != 1 ||
		got["seasons"].([]any)[0].(map[string]any)["season"] != "2026-09-01" || got["last"] != nil ||
		len(got["market"].([]any)) != 1 || len(got["achievements"].([]any)) != 0 {
		t.Fatalf("arena: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/pets/arena", `{"action":"bet","petId":"p3","votes":100}`, auth...)
	got = decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.pets.arena != (model.PetArenaAction{Action: "bet", PetID: "p3", Votes: 100}) ||
		got["balance"] != 77.0 || got["bout"] == nil || got["arena"].(map[string]any)["boss"] == nil {
		t.Fatalf("bet: %d %v", w.Code, got)
	}
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
	w := h.do("GET", "/api/v1/me/pets", "", auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || len(got["pets"].([]any)) != 0 || got["shop"].(map[string]any)["meal"] != 5.0 {
		t.Fatalf("no pet: %d %v", w.Code, got)
	}

	h.pets.noPet = false
	w = h.do("GET", "/api/v1/me/pets", "", auth...)
	if pets := decode[map[string]any](t, w)["pets"].([]any); len(pets) != 1 || pets[0].(map[string]any)["id"] != "p1" {
		t.Fatalf("pets: %v", pets)
	}

	w = h.do("POST", "/api/v1/me/pets", `{"kind":"cat","name":"Барсик"}`, auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusCreated || h.pets.kind != "cat" || h.pets.name != "Барсик" ||
		got["balance"] != 900.0 || got["pet"].(map[string]any)["rating"] != 1016.0 || got["pet"].(map[string]any)["playAt"] != nil {
		t.Fatalf("adopt: %d %v", w.Code, got)
	}

	if w = h.do("POST", "/api/v1/me/pets/p1/play", "", auth...); decode[map[string]any](t, w)["pet"].(map[string]any)["playAt"] != "2026-10-26T13:00:00Z" {
		t.Fatalf("play: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/me/pets/p1/feed", "", auth...); w.Code != http.StatusOK || h.pets.petID != "p1" {
		t.Fatalf("feed: %d", w.Code)
	}
	if w = h.do("PATCH", "/api/v1/me/pets/p2", `{"name":"Кекс"}`, auth...); w.Code != http.StatusOK || h.pets.name != "Кекс" || h.pets.petID != "p2" {
		t.Fatalf("rename: %d", w.Code)
	}

	// The defender's page is deleted: the fight shows no owner of it.
	w = h.do("POST", "/api/v1/me/pets/p1/fights", `{"nickname":"cat","stance":"agility"}`, auth...)
	fought := decode[struct {
		Fight struct {
			Attacker map[string]any   `json:"attacker"`
			Defender map[string]any   `json:"defender"`
			Hits     []map[string]any `json:"hits"`
			Won      bool             `json:"won"`
		} `json:"fight"`
		Balance int64 `json:"balance"`
	}](t, w)
	if w.Code != http.StatusOK || h.pets.nickname != "cat" || h.pets.stance != "agility" || fought.Balance != 903 || !fought.Fight.Won ||
		fought.Fight.Attacker["user"].(map[string]any)["nickname"] != "ann" || fought.Fight.Defender["user"] != nil ||
		fought.Fight.Hits[0]["crit"] != true {
		t.Fatalf("fight: %d %+v", w.Code, fought)
	}

	if w = h.do("GET", "/api/v1/me/pets/fights", "", auth...); len(decode[map[string]any](t, w)["fights"].([]any)) != 1 {
		t.Fatalf("fights: %d", w.Code)
	}
	if w = h.do("GET", "/api/v1/users/bob/pets", ""); w.Code != http.StatusOK || decode[map[string]any](t, w)["pets"].([]any)[0].(map[string]any)["name"] != "Барсик" {
		t.Fatalf("a pet of a page: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/pet/settings", "", auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["spent"] != 155.0 || got["shop"].(map[string]any)["fights"] != 10.0 {
		t.Fatalf("settings: %d %v", w.Code, got)
	}
	w = h.do("PUT", "/api/v1/pet/settings", `{"prices":{"cat":10,"dog":20,"hamster":5,"dragon":50},"cases":{"bronze":1,"silver":2,"gold":3},"meal":1,"win":2,"fights":3}`, auth...)
	if w.Code != http.StatusNoContent || h.pets.shop.Prices["dragon"] != 50 || h.pets.shop.Fights != 3 || h.pets.shop.Cases["gold"] != 3 {
		t.Fatalf("set: %d %+v", w.Code, h.pets.shop)
	}

	// The catalogue, a case, clothes bought and taken off.
	w = h.do("GET", "/api/v1/me/pets", "", auth...)
	catalogue := decode[struct {
		Shop    map[string]any   `json:"shop"`
		Kinds   []map[string]any `json:"kinds"`
		Cases   []map[string]any `json:"cases"`
		Items   []map[string]any `json:"items"`
		Pets    []map[string]any `json:"pets"`
		Keeper  map[string]any   `json:"keeper"`
		Battles []map[string]any `json:"battles"`
	}](t, w)
	if catalogue.Keeper["dropKind"] != "owl" || catalogue.Cases[0]["shards"] != 60.0 || len(catalogue.Battles) != 1 ||
		catalogue.Battles[0]["defender"].(map[string]any)["user"].(map[string]any)["nickname"] != "bob" {
		t.Fatalf("the keeper and the fights: %+v", catalogue)
	}
	if catalogue.Shop["cases"].(map[string]any)["gold"] != 1200.0 || catalogue.Kinds[0]["rarity"] != "legendary" ||
		len(catalogue.Cases[0]["odds"].([]any)) != 4 || catalogue.Items[0]["bonus"].(map[string]any)["defense"] != 3.0 ||
		len(catalogue.Pets[0]["outfit"].([]any)) != 0 {
		t.Fatalf("catalogue: %+v", catalogue)
	}
	w = h.do("POST", "/api/v1/me/pets/cases", `{"case":"gold","shards":true}`, auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || h.pets.kind != "gold" || !h.pets.shards || got["balance"] != 300.0 ||
		got["keeper"].(map[string]any)["dropKind"] != "fox" {
		t.Fatalf("open a case: %d %v", w.Code, got)
	}
	if w = h.do("POST", "/api/v1/me/pets/drop", `{"name":"Находка"}`, auth...); w.Code != http.StatusCreated || h.pets.name != "Находка" {
		t.Fatalf("take the pet: %d", w.Code)
	}
	if w = h.do("DELETE", "/api/v1/me/pets/drop", "", auth...); w.Code != http.StatusOK || decode[map[string]any](t, w)["keeper"].(map[string]any)["shards"] != 3.0 {
		t.Fatalf("leave the pet: %d", w.Code)
	}

	// Stances and a fight led a round after another.
	if w = h.do("PUT", "/api/v1/me/pets/p1/stance", `{"stance":"agility"}`, auth...); w.Code != http.StatusOK ||
		decode[map[string]any](t, w)["pet"].(map[string]any)["stance"] != "agility" {
		t.Fatalf("stance: %d", w.Code)
	}
	w = h.do("POST", "/api/v1/me/pets/p1/battle", `{"nickname":"bob","stance":"attack"}`, auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusCreated || h.pets.stance != "attack" || got["fight"] != nil ||
		got["battle"].(map[string]any)["hits"].([]any)[0].(map[string]any)["move"] != "guard" {
		t.Fatalf("start a fight: %d %v", w.Code, got)
	}
	w = h.do("POST", "/api/v1/me/pets/p1/battle/moves", `{"move":"power"}`, auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || h.pets.move != "power" || got["fight"] == nil || got["balance"] != 905.0 {
		t.Fatalf("a move: %d %v", w.Code, got)
	}
	w = h.do("POST", "/api/v1/me/pets/p1/clothes", `{"item":"scarf"}`, auth...)
	if got := decode[map[string]any](t, w)["pet"].(map[string]any); w.Code != http.StatusOK || h.pets.item != "scarf" ||
		got["outfit"].([]any)[0] != "scarf" || got["clothes"].(map[string]any)["health"] != 12.0 {
		t.Fatalf("buy clothes: %d %v", w.Code, got)
	}
	if w = h.do("PUT", "/api/v1/me/pets/p1/outfit", `{"item":"scarf","wear":true}`, auth...); w.Code != http.StatusOK || !h.pets.wear || h.pets.petID != "p1" {
		t.Fatalf("dress: %d", w.Code)
	}

	if w = h.do("DELETE", "/api/v1/me/pets/p1", "", auth...); w.Code != http.StatusNoContent || h.pets.petID != "p1" {
		t.Fatalf("release: %d", w.Code)
	}

	h.pets.err = failure.New(failure.Conflict, "the pet is hungry")
	if w = h.do("POST", "/api/v1/me/pets/p1/fights", `{}`, auth...); w.Code != http.StatusConflict {
		t.Fatalf("a hungry pet: %d", w.Code)
	}
}
