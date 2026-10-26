package rest

import (
	"context"
	"net/http"
	"slices"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Pets are "Гости", "Рейтинги" and "Питомец", which the user service
// keeps with the people and their votes.
type Pets interface {
	Visit(ctx context.Context, accessToken, nickname string) error
	Guests(ctx context.Context, accessToken, cursor string, limit int) (model.GuestPage, error)
	Top(ctx context.Context, accessToken, board string) ([]model.TopEntry, error)
	Get(ctx context.Context, accessToken string) (model.PetState, error)
	UserPet(ctx context.Context, accessToken, nickname string) (model.Pet, string, error)
	Adopt(ctx context.Context, accessToken, kind, name string) (model.PetCare, error)
	Feed(ctx context.Context, accessToken string) (model.PetCare, error)
	Play(ctx context.Context, accessToken string) (model.PetCare, error)
	Rename(ctx context.Context, accessToken, name string) (model.PetCare, error)
	Release(ctx context.Context, accessToken string) error
	Fight(ctx context.Context, accessToken, nickname string) (model.PetFought, error)
	Fights(ctx context.Context, accessToken string) ([]model.PetFight, error)
	Settings(ctx context.Context, accessToken string) (model.PetShop, model.PetStats, error)
	Set(ctx context.Context, accessToken string, shop model.PetShop) error
}

type petShopJSON struct {
	Prices map[string]int64 `json:"prices"`
	Meal   int64            `json:"meal"`
	Win    int64            `json:"win"`
	Fights int              `json:"fights"`
}

// petJSON is a pet as it is now; playAt is when it may play again (null —
// now), nextLevelExperience 0 at the last level.
type petJSON struct {
	Kind                string  `json:"kind"`
	Name                string  `json:"name"`
	BornAt              string  `json:"bornAt"`
	Satiety             int     `json:"satiety"`
	Joy                 int     `json:"joy"`
	PlayAt              *string `json:"playAt"`
	Experience          int64   `json:"experience"`
	Level               int     `json:"level"`
	LevelExperience     int64   `json:"levelExperience"`
	NextLevelExperience int64   `json:"nextLevelExperience"`
	Rating              int     `json:"rating"`
	Wins                int     `json:"wins"`
	Losses              int     `json:"losses"`
	Fights              int     `json:"fights"`
	Attack              int     `json:"attack"`
	Defense             int     `json:"defense"`
	Health              int     `json:"health"`
	Speed               int     `json:"speed"`
	Crit                int     `json:"crit"`
}

// petFighterJSON is a pet as it came to a fight, with its owner — null
// when their page is deleted or erased.
type petFighterJSON struct {
	User   *authorJSON `json:"user"`
	Kind   string      `json:"kind"`
	Name   string      `json:"name"`
	Level  int         `json:"level"`
	Health int         `json:"health"`
}

type petHitJSON struct {
	ByAttacker     bool `json:"byAttacker"`
	Dodged         bool `json:"dodged"`
	Crit           bool `json:"crit"`
	Damage         int  `json:"damage"`
	AttackerHealth int  `json:"attackerHealth"`
	DefenderHealth int  `json:"defenderHealth"`
}

type petFightJSON struct {
	ID       string         `json:"id"`
	FoughtAt string         `json:"foughtAt"`
	Attacker petFighterJSON `json:"attacker"`
	Defender petFighterJSON `json:"defender"`
	Hits     []petHitJSON   `json:"hits"`
	Won      bool           `json:"won"`
	Rating   int            `json:"rating"`
}

func toPetShopJSON(s model.PetShop) petShopJSON {
	prices := s.Prices
	if prices == nil {
		prices = map[string]int64{}
	}

	return petShopJSON{Prices: prices, Meal: s.Meal, Win: s.Win, Fights: s.Fights}
}

func toPetJSON(p model.Pet) petJSON {
	return petJSON{
		Kind:                p.Kind,
		Name:                p.Name,
		BornAt:              timeString(p.BornAt),
		Satiety:             p.Satiety,
		Joy:                 p.Joy,
		PlayAt:              optionalTime(p.PlayAt),
		Experience:          p.Experience,
		Level:               p.Level,
		LevelExperience:     p.LevelExperience,
		NextLevelExperience: p.NextLevelExperience,
		Rating:              p.Rating,
		Wins:                p.Wins,
		Losses:              p.Losses,
		Fights:              p.Fights,
		Attack:              p.Attack,
		Defense:             p.Defense,
		Health:              p.Health,
		Speed:               p.Speed,
		Crit:                p.Crit,
	}
}

func toPetFighterJSON(f model.PetFighter, owner string, people map[string]model.Author) petFighterJSON {
	out := petFighterJSON{Kind: f.Kind, Name: f.Name, Level: f.Level, Health: f.Health}
	if a, ok := people[owner]; ok && !a.Deactivated {
		j := toAuthorJSON(a)
		out.User = &j
	}

	return out
}

func toPetFightJSON(f model.PetFight, people map[string]model.Author) petFightJSON {
	out := petFightJSON{
		ID:       f.ID,
		FoughtAt: timeString(f.FoughtAt),
		Attacker: toPetFighterJSON(f.Attacker, f.AttackerID, people),
		Defender: toPetFighterJSON(f.Defender, f.DefenderID, people),
		Hits:     make([]petHitJSON, 0, len(f.Hits)),
		Won:      f.Won,
		Rating:   f.Rating,
	}
	for _, h := range f.Hits {
		out.Hits = append(out.Hits, petHitJSON(h))
	}

	return out
}

// fightPeople are the owners of the pets of fights.
func (a *api) fightPeople(r *http.Request, fights ...model.PetFight) (map[string]model.Author, error) {
	ids := make([]string, 0, 2*len(fights))
	for _, f := range fights {
		ids = append(ids, f.AttackerID, f.DefenderID)
	}

	return a.people(r, ids)
}

// profiles are the people of ids as lists show them; those of deleted
// pages and erased are missing.
func (a *api) profiles(r *http.Request, ids []string) (map[string]model.PublicProfile, error) {
	byID := make(map[string]model.PublicProfile, len(ids))

	for chunk := range slices.Chunk(unique(ids), maxProfilesPerRequest) {
		found, err := a.users.GetProfiles(r.Context(), chunk)
		if err != nil {
			return nil, err
		}

		for _, p := range found {
			if !p.Deactivated {
				byID[p.ID] = p
			}
		}
	}

	return byID, nil
}

// visitPage leaves the caller among the guests of a page: the page tells
// it when it opens.
func (a *api) visitPage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.pets.Visit(r.Context(), token, r.PathValue("nickname")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// guestJSON is a guest of the page; new: they came since the owner last
// looked.
type guestJSON struct {
	User      publicProfileResponse `json:"user"`
	VisitedAt string                `json:"visitedAt"`
	New       bool                  `json:"new"`
}

// listGuests is "Гости" of the caller: who came, the last visit first.
func (a *api) listGuests(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.pets.Guests(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(page.Guests))
	for _, g := range page.Guests {
		ids = append(ids, g.UserID)
	}

	people, err := a.profiles(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	guests := make([]guestJSON, 0, len(page.Guests))
	for _, g := range page.Guests {
		if p, ok := people[g.UserID]; ok {
			guests = append(guests, guestJSON{User: toPublicProfileResponse(p), VisitedAt: timeString(g.VisitedAt), New: g.VisitedAt.After(page.SeenAt)})
		}
	}

	writeJSON(w, http.StatusOK, struct {
		Guests     []guestJSON `json:"guests"`
		SeenAt     *string     `json:"seenAt"`
		NextCursor string      `json:"nextCursor"`
	}{guests, optionalTime(page.SeenAt), page.NextCursor})
}

// topEntryJSON is a place of a board: votes, or the rating of a pet with
// the pet (null on the other boards).
type topEntryJSON struct {
	Place int                   `json:"place"`
	User  publicProfileResponse `json:"user"`
	Value int64                 `json:"value"`
	Pet   *struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"pet"`
}

// listTop is a board of "Рейтинги": "rich", "miners", "business",
// "casino" or "pets"; public.
func (a *api) listTop(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	entries, err := a.pets.Top(r.Context(), token, r.URL.Query().Get("board"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.UserID)
	}

	people, err := a.profiles(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := make([]topEntryJSON, 0, len(entries))
	for _, e := range entries {
		p, ok := people[e.UserID]
		if !ok {
			continue
		}

		entry := topEntryJSON{Place: len(out) + 1, User: toPublicProfileResponse(p), Value: e.Value}
		if e.PetKind != "" {
			entry.Pet = &struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			}{e.PetKind, e.PetName}
		}
		out = append(out, entry)
	}

	writeJSON(w, http.StatusOK, struct {
		Entries []topEntryJSON `json:"entries"`
	}{out})
}

// getPet is "Питомец" of the caller: the shop and the pet, null without
// one.
func (a *api) getPet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	state, err := a.pets.Get(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := struct {
		Shop petShopJSON `json:"shop"`
		Pet  *petJSON    `json:"pet"`
	}{Shop: toPetShopJSON(state.Shop)}
	if state.Pet != nil {
		pet := toPetJSON(*state.Pet)
		resp.Pet = &pet
	}

	writeJSON(w, http.StatusOK, resp)
}

// getUserPet is the pet of a page, public as the page.
func (a *api) getUserPet(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	pet, _, err := a.pets.UserPet(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toPetJSON(pet))
}

type petCareJSON struct {
	Pet     petJSON `json:"pet"`
	Balance int64   `json:"balance"`
}

func (a *api) adoptPet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePetCare(w, r, func() (model.PetCare, error) { return a.pets.Adopt(r.Context(), token, req.Kind, req.Name) }, http.StatusCreated)
}

func (a *api) feedPet(w http.ResponseWriter, r *http.Request) {
	a.carePet(w, r, a.pets.Feed)
}

func (a *api) playPet(w http.ResponseWriter, r *http.Request) {
	a.carePet(w, r, a.pets.Play)
}

func (a *api) carePet(w http.ResponseWriter, r *http.Request, care func(ctx context.Context, accessToken string) (model.PetCare, error)) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePetCare(w, r, func() (model.PetCare, error) { return care(r.Context(), token) }, http.StatusOK)
}

func (a *api) renamePet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePetCare(w, r, func() (model.PetCare, error) { return a.pets.Rename(r.Context(), token, req.Name) }, http.StatusOK)
}

func (a *api) writePetCare(w http.ResponseWriter, r *http.Request, care func() (model.PetCare, error), code int) {
	c, err := care()
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, code, petCareJSON{Pet: toPetJSON(c.Pet), Balance: c.Balance})
}

func (a *api) releasePet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.pets.Release(r.Context(), token); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// fightPet sets the caller's pet on the pet of a page, or on one of a
// near rating without a nickname.
func (a *api) fightPet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Nickname string `json:"nickname"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	fought, err := a.pets.Fight(r.Context(), token, req.Nickname)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.fightPeople(r, fought.Fight)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Fight   petFightJSON `json:"fight"`
		Pet     petJSON      `json:"pet"`
		Balance int64        `json:"balance"`
	}{toPetFightJSON(fought.Fight, people), toPetJSON(fought.Pet), fought.Balance})
}

// listPetFights are the last fights of the caller's pet, newest first.
func (a *api) listPetFights(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	fights, err := a.pets.Fights(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.fightPeople(r, fights...)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := make([]petFightJSON, 0, len(fights))
	for _, f := range fights {
		out = append(out, toPetFightJSON(f, people))
	}

	writeJSON(w, http.StatusOK, struct {
		Fights []petFightJSON `json:"fights"`
	}{out})
}

// petSettings is "Питомцы" of the admin panel: the shop and how the pets
// do.
func (a *api) petSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shop, stats, err := a.pets.Settings(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Shop   petShopJSON `json:"shop"`
		Pets   int64       `json:"pets"`
		Fights int64       `json:"fights"`
		Spent  int64       `json:"spent"`
		Won    int64       `json:"won"`
	}{toPetShopJSON(shop), stats.Pets, stats.Fights, stats.Spent, stats.Won})
}

func (a *api) setPetSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req petShopJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.pets.Set(r.Context(), token, model.PetShop(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
