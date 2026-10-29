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
	UserPets(ctx context.Context, accessToken, nickname string) ([]model.Pet, string, error)
	Adopt(ctx context.Context, accessToken, kind, name string) (model.PetCare, error)
	Feed(ctx context.Context, accessToken, petID string) (model.PetCare, error)
	Play(ctx context.Context, accessToken, petID string) (model.PetCare, error)
	Rename(ctx context.Context, accessToken, petID, name string) (model.PetCare, error)
	Release(ctx context.Context, accessToken, petID string) error
	SetStance(ctx context.Context, accessToken, petID, stance string) (model.PetCare, error)
	Fight(ctx context.Context, accessToken, petID, nickname, stance string) (model.PetFought, error)
	Fights(ctx context.Context, accessToken string) ([]model.PetFight, error)
	StartBattle(ctx context.Context, accessToken, petID, nickname, stance string) (model.PetBattleTurn, error)
	MoveBattle(ctx context.Context, accessToken, petID, move string) (model.PetBattleTurn, error)
	OpenCase(ctx context.Context, accessToken, caseID string, withShards bool) (model.PetDrop, error)
	TakeDrop(ctx context.Context, accessToken, name string) (model.PetCare, error)
	LeaveDrop(ctx context.Context, accessToken string) (model.PetKeeper, error)
	BuyItem(ctx context.Context, accessToken, petID, item string) (model.PetCare, error)
	Dress(ctx context.Context, accessToken, petID, item string, wear bool) (model.PetCare, error)
	Settings(ctx context.Context, accessToken string) (model.PetShop, model.PetStats, error)
	Set(ctx context.Context, accessToken string, shop model.PetShop) error
}

// petShopJSON is the shop; cases left out of a change keep their prices.
type petShopJSON struct {
	Prices map[string]int64 `json:"prices"`
	Cases  map[string]int64 `json:"cases"`
	Meal   int64            `json:"meal"`
	Win    int64            `json:"win"`
	Fights int              `json:"fights"`
}

type petPowerJSON struct {
	Attack  int `json:"attack"`
	Defense int `json:"defense"`
	Health  int `json:"health"`
	Speed   int `json:"speed"`
}

// petKindJSON is a kind of the catalogue; inShop: the shop sells it, the
// others fall out of cases. shards are what it leaves when let go.
type petKindJSON struct {
	Kind    string       `json:"kind"`
	Rarity  string       `json:"rarity"`
	InShop  bool         `json:"inShop"`
	Stats   petPowerJSON `json:"stats"`
	Crit    int          `json:"crit"`
	Element string       `json:"element"`
	Special string       `json:"special"`
	Shards  int          `json:"shards"`
}

// petCaseJSON is a case: the odds in a thousand of common, rare, epic and
// legendary pets, and its price in shards.
type petCaseJSON struct {
	ID     string `json:"id"`
	Odds   []int  `json:"odds"`
	Shards int    `json:"shards"`
}

// petKeeperJSON is what the caller keeps of the cases; dropKind and
// dropCase are the pet a case gave, "" when none waits.
type petKeeperJSON struct {
	Shards     int    `json:"shards"`
	Pity       int    `json:"pity"`
	PityLimit  int    `json:"pityLimit"`
	Cases      int    `json:"cases"`
	CasesLimit int    `json:"casesLimit"`
	DropKind   string `json:"dropKind"`
	DropCase   string `json:"dropCase"`
}

type petItemJSON struct {
	ID    string       `json:"id"`
	Slot  string       `json:"slot"`
	Price int64        `json:"price"`
	Bonus petPowerJSON `json:"bonus"`
}

// petJSON is a pet as it is now; playAt is when it may play again (null —
// now), nextLevelExperience 0 at the last level.
type petJSON struct {
	ID                  string       `json:"id"`
	Kind                string       `json:"kind"`
	Name                string       `json:"name"`
	BornAt              string       `json:"bornAt"`
	Satiety             int          `json:"satiety"`
	Joy                 int          `json:"joy"`
	PlayAt              *string      `json:"playAt"`
	Experience          int64        `json:"experience"`
	Level               int          `json:"level"`
	LevelExperience     int64        `json:"levelExperience"`
	NextLevelExperience int64        `json:"nextLevelExperience"`
	Rating              int          `json:"rating"`
	Wins                int          `json:"wins"`
	Losses              int          `json:"losses"`
	Fights              int          `json:"fights"`
	Attack              int          `json:"attack"`
	Defense             int          `json:"defense"`
	Health              int          `json:"health"`
	Speed               int          `json:"speed"`
	Crit                int          `json:"crit"`
	Rarity              string       `json:"rarity"`
	Wardrobe            []string     `json:"wardrobe"`
	Outfit              []string     `json:"outfit"`
	Breed               petPowerJSON `json:"breed"`
	Clothes             petPowerJSON `json:"clothes"`
	Stance              string       `json:"stance"`
	Power               int          `json:"power"`
	Fatigue             int          `json:"fatigue"`
}

// petFighterJSON is a pet as it came to a fight, with its owner — null
// when their page is deleted or erased.
type petFighterJSON struct {
	User    *authorJSON `json:"user"`
	Kind    string      `json:"kind"`
	Name    string      `json:"name"`
	Level   int         `json:"level"`
	Health  int         `json:"health"`
	Outfit  []string    `json:"outfit"`
	Stance  string      `json:"stance"`
	Power   int         `json:"power"`
	Fatigue int         `json:"fatigue"`
}

type petHitJSON struct {
	ByAttacker     bool   `json:"byAttacker"`
	Move           string `json:"move"`
	Dodged         bool   `json:"dodged"`
	Crit           bool   `json:"crit"`
	Damage         int    `json:"damage"`
	Heal           int    `json:"heal"`
	Special        string `json:"special"`
	Answer         string `json:"answer"`
	AttackerHealth int    `json:"attackerHealth"`
	DefenderHealth int    `json:"defenderHealth"`
}

type petFightJSON struct {
	ID       string         `json:"id"`
	FoughtAt string         `json:"foughtAt"`
	Attacker petFighterJSON `json:"attacker"`
	Defender petFighterJSON `json:"defender"`
	Hits     []petHitJSON   `json:"hits"`
	Won      bool           `json:"won"`
	Rating   int            `json:"rating"`
	Ratio    int            `json:"ratio"`
	Reward   int64          `json:"reward"`
}

// petBattleJSON is a fight the caller leads a round after another: win
// is the rating a win brings, loss what a defeat costs.
type petBattleJSON struct {
	PetID          string         `json:"petId"`
	Attacker       petFighterJSON `json:"attacker"`
	Defender       petFighterJSON `json:"defender"`
	Hits           []petHitJSON   `json:"hits"`
	Round          int            `json:"round"`
	Rounds         int            `json:"rounds"`
	AttackerHealth int            `json:"attackerHealth"`
	DefenderHealth int            `json:"defenderHealth"`
	Ratio          int            `json:"ratio"`
	Win            int            `json:"win"`
	Loss           int            `json:"loss"`
}

func toPetShopJSON(s model.PetShop) petShopJSON {
	prices := s.Prices
	if prices == nil {
		prices = map[string]int64{}
	}

	cases := s.Cases
	if cases == nil {
		cases = map[string]int64{}
	}

	return petShopJSON{Prices: prices, Cases: cases, Meal: s.Meal, Win: s.Win, Fights: s.Fights}
}

// names are list, [] for none.
func names(list []string) []string {
	if list == nil {
		return []string{}
	}

	return list
}

func toPetJSON(p model.Pet) petJSON {
	return petJSON{
		ID:                  p.ID,
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
		Rarity:              p.Rarity,
		Wardrobe:            names(p.Wardrobe),
		Outfit:              names(p.Outfit),
		Breed:               petPowerJSON(p.Breed),
		Clothes:             petPowerJSON(p.Clothes),
		Stance:              p.Stance,
		Power:               p.Power,
		Fatigue:             p.Fatigue,
	}
}

func toPetsJSON(pets []model.Pet) []petJSON {
	out := make([]petJSON, 0, len(pets))
	for _, p := range pets {
		out = append(out, toPetJSON(p))
	}

	return out
}

func toPetFighterJSON(f model.PetFighter, owner string, people map[string]model.Author) petFighterJSON {
	out := petFighterJSON{
		Kind: f.Kind, Name: f.Name, Level: f.Level, Health: f.Health, Outfit: names(f.Outfit),
		Stance: f.Stance, Power: f.Power, Fatigue: f.Fatigue,
	}
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
		Hits:     toPetHitsJSON(f.Hits),
		Won:      f.Won,
		Rating:   f.Rating,
		Ratio:    f.Ratio,
		Reward:   f.Reward,
	}

	return out
}

func toPetHitsJSON(hits []model.PetHit) []petHitJSON {
	out := make([]petHitJSON, 0, len(hits))
	for _, h := range hits {
		out = append(out, petHitJSON(h))
	}

	return out
}

// toPetBattleJSON is b of the caller me.
func toPetBattleJSON(b model.PetBattle, me string, people map[string]model.Author) petBattleJSON {
	return petBattleJSON{
		PetID:          b.PetID,
		Attacker:       toPetFighterJSON(b.Attacker, me, people),
		Defender:       toPetFighterJSON(b.Defender, b.DefenderID, people),
		Hits:           toPetHitsJSON(b.Hits),
		Round:          b.Round,
		Rounds:         b.Rounds,
		AttackerHealth: b.AttackerHealth,
		DefenderHealth: b.DefenderHealth,
		Ratio:          b.Ratio,
		Win:            b.Win,
		Loss:           b.Loss,
	}
}

func toPetKeeperJSON(k model.PetKeeper) petKeeperJSON {
	return petKeeperJSON(k)
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

// getPets is "Питомцы" of the caller: the shop and the pets, the oldest
// first.
func (a *api) getPets(w http.ResponseWriter, r *http.Request) {
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

	kinds := make([]petKindJSON, 0, len(state.Kinds))
	for _, k := range state.Kinds {
		kinds = append(kinds, petKindJSON{
			Kind: k.Kind, Rarity: k.Rarity, InShop: k.InShop, Stats: petPowerJSON(k.Stats), Crit: k.Crit,
			Element: k.Element, Special: k.Special, Shards: k.Shards,
		})
	}
	cases := make([]petCaseJSON, 0, len(state.Cases))
	for _, c := range state.Cases {
		cases = append(cases, petCaseJSON(c))
	}
	items := make([]petItemJSON, 0, len(state.Items))
	for _, item := range state.Items {
		items = append(items, petItemJSON{ID: item.ID, Slot: item.Slot, Price: item.Price, Bonus: petPowerJSON(item.Bonus)})
	}

	battles, err := a.battlesJSON(r, state.Battles)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Shop    petShopJSON     `json:"shop"`
		Pets    []petJSON       `json:"pets"`
		Kinds   []petKindJSON   `json:"kinds"`
		Cases   []petCaseJSON   `json:"cases"`
		Items   []petItemJSON   `json:"items"`
		Keeper  petKeeperJSON   `json:"keeper"`
		Battles []petBattleJSON `json:"battles"`
	}{toPetShopJSON(state.Shop), toPetsJSON(state.Pets), kinds, cases, items, toPetKeeperJSON(state.Keeper), battles})
}

// battlesJSON are the fights of the caller with the owners of the pets
// they fight; the caller themself is left out, the page knows them.
func (a *api) battlesJSON(r *http.Request, battles []model.PetBattle) ([]petBattleJSON, error) {
	ids := make([]string, 0, len(battles))
	for _, b := range battles {
		ids = append(ids, b.DefenderID)
	}

	people, err := a.people(r, ids)
	if err != nil {
		return nil, err
	}

	out := make([]petBattleJSON, 0, len(battles))
	for _, b := range battles {
		out = append(out, toPetBattleJSON(b, "", people))
	}

	return out, nil
}

// getUserPets are the pets of a page, public as the page.
func (a *api) getUserPets(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	pets, _, err := a.pets.UserPets(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Pets []petJSON `json:"pets"`
	}{toPetsJSON(pets)})
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

// openPetCase opens a case, for votes or for shards: the pet that fell
// out of it waits to be taken or left.
func (a *api) openPetCase(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Case   string `json:"case"`
		Shards bool   `json:"shards"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	drop, err := a.pets.OpenCase(r.Context(), token, req.Case, req.Shards)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Keeper  petKeeperJSON `json:"keeper"`
		Balance int64         `json:"balance"`
	}{toPetKeeperJSON(drop.Keeper), drop.Balance})
}

// takePetDrop takes the pet a case gave, called name.
func (a *api) takePetDrop(w http.ResponseWriter, r *http.Request) {
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

	a.writePetCare(w, r, func() (model.PetCare, error) { return a.pets.TakeDrop(r.Context(), token, req.Name) }, http.StatusCreated)
}

// leavePetDrop leaves the pet a case gave for its shards.
func (a *api) leavePetDrop(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	keeper, err := a.pets.LeaveDrop(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Keeper petKeeperJSON `json:"keeper"`
	}{toPetKeeperJSON(keeper)})
}

// setPetStance sets the stance a pet defends in.
func (a *api) setPetStance(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Stance string `json:"stance"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePetCare(w, r, func() (model.PetCare, error) {
		return a.pets.SetStance(r.Context(), token, r.PathValue("id"), req.Stance)
	}, http.StatusOK)
}

// buyPetItem buys clothes for a pet and puts them on.
func (a *api) buyPetItem(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Item string `json:"item"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePetCare(w, r, func() (model.PetCare, error) { return a.pets.BuyItem(r.Context(), token, r.PathValue("id"), req.Item) }, http.StatusOK)
}

// dressPet puts clothes of the wardrobe on a pet, or takes them off.
func (a *api) dressPet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Item string `json:"item"`
		Wear bool   `json:"wear"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePetCare(w, r, func() (model.PetCare, error) {
		return a.pets.Dress(r.Context(), token, r.PathValue("id"), req.Item, req.Wear)
	}, http.StatusOK)
}

func (a *api) feedPet(w http.ResponseWriter, r *http.Request) {
	a.carePet(w, r, a.pets.Feed)
}

func (a *api) playPet(w http.ResponseWriter, r *http.Request) {
	a.carePet(w, r, a.pets.Play)
}

func (a *api) carePet(w http.ResponseWriter, r *http.Request, care func(ctx context.Context, accessToken, petID string) (model.PetCare, error)) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePetCare(w, r, func() (model.PetCare, error) { return care(r.Context(), token, r.PathValue("id")) }, http.StatusOK)
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

	a.writePetCare(w, r, func() (model.PetCare, error) { return a.pets.Rename(r.Context(), token, r.PathValue("id"), req.Name) }, http.StatusOK)
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

	if err = a.pets.Release(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// fightPet sets a pet of the caller on the best pet of a page, or on one
// of a near rating without a nickname.
func (a *api) fightPet(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req fightRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	fought, err := a.pets.Fight(r.Context(), token, r.PathValue("id"), req.Nickname, req.Stance)
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

// fightRequest is whom a pet fights, the best pet of a page or one near
// in power without a nickname, and in which stance, "" for the one it
// defends in.
type fightRequest struct {
	Nickname string `json:"nickname"`
	Stance   string `json:"stance"`
}

// startPetBattle starts a fight the caller leads a round after another.
func (a *api) startPetBattle(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req fightRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeBattleTurn(w, r, func() (model.PetBattleTurn, error) {
		return a.pets.StartBattle(r.Context(), token, r.PathValue("id"), req.Nickname, req.Stance)
	}, http.StatusCreated)
}

// movePetBattle makes a move in the fight of a pet: "strike", "guard",
// "power" or "auto" for the rest of it.
func (a *api) movePetBattle(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Move string `json:"move"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeBattleTurn(w, r, func() (model.PetBattleTurn, error) {
		return a.pets.MoveBattle(r.Context(), token, r.PathValue("id"), req.Move)
	}, http.StatusOK)
}

// writeBattleTurn writes the fight after turn, the pet, and the fight as
// the list keeps it once it is over (null before).
func (a *api) writeBattleTurn(w http.ResponseWriter, r *http.Request, turn func() (model.PetBattleTurn, error), code int) {
	t, err := turn()
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := []string{t.Battle.DefenderID}
	if t.Fight != nil {
		ids = append(ids, t.Fight.AttackerID)
	}

	people, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var fight *petFightJSON
	if t.Fight != nil {
		f := toPetFightJSON(*t.Fight, people)
		fight = &f
	}

	writeJSON(w, code, struct {
		Battle  petBattleJSON `json:"battle"`
		Pet     petJSON       `json:"pet"`
		Fight   *petFightJSON `json:"fight"`
		Balance int64         `json:"balance"`
	}{toPetBattleJSON(t.Battle, "", people), toPetJSON(t.Pet), fight, t.Balance})
}

// listPetFights are the last fights of the caller's pets, newest first.
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
