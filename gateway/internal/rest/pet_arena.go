package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// petArenaJSON is the arena of the pets; a person is null when their page
// is deleted or erased.
type petArenaJSON struct {
	Boss         petBossJSON          `json:"boss"`
	Tournament   petTournamentJSON    `json:"tournament"`
	Last         *petTournamentJSON   `json:"last"`
	SeasonEndsAt string               `json:"seasonEndsAt"`
	Top          []petSeasonPlaceJSON `json:"top"`
	Seasons      []petSeasonJSON      `json:"seasons"`
	Quests       []petQuestJSON       `json:"quests"`
	Streak       int                  `json:"streak"`
	Achievements []petAchievementJSON `json:"achievements"`
	Market       []petListingJSON     `json:"market"`
}

type petBossJSON struct {
	Kind      string           `json:"kind"`
	Health    int              `json:"health"`
	MaxHealth int              `json:"maxHealth"`
	KilledBy  *authorJSON      `json:"killedBy"`
	KilledAt  *string          `json:"killedAt"`
	Hits      []petBossHitJSON `json:"hits"`
	Stats     petPowerJSON     `json:"stats"`
	Prize     int64            `json:"prize"`
	LastHit   int64            `json:"lastHit"`
}

type petBossHitJSON struct {
	User   *authorJSON `json:"user"`
	Damage int         `json:"damage"`
	Hits   int         `json:"hits"`
}

type petTournamentJSON struct {
	StartsAt string           `json:"startsAt"`
	Held     bool             `json:"held"`
	Entrants []petEntrantJSON `json:"entrants"`
	Rounds   [][]petMatchJSON `json:"rounds"`
	Places   []int            `json:"places"`
	Bets     int              `json:"bets"`
	Pool     int64            `json:"pool"`
	BetPetID string           `json:"betPetId"`
	BetVotes int64            `json:"betVotes"`
}

type petEntrantJSON struct {
	User  *authorJSON `json:"user"`
	PetID string      `json:"petId"`
	Kind  string      `json:"kind"`
	Name  string      `json:"name"`
	Level int         `json:"level"`
	Power int         `json:"power"`
}

type petMatchJSON struct {
	A      int           `json:"a"`
	B      int           `json:"b"`
	Winner int           `json:"winner"`
	Bout   *petFightJSON `json:"bout"`
}

type petSeasonPlaceJSON struct {
	User   *authorJSON `json:"user"`
	PetID  string      `json:"petId"`
	Kind   string      `json:"kind"`
	Name   string      `json:"name"`
	Rating int         `json:"rating"`
	Prize  int64       `json:"prize"`
}

type petSeasonJSON struct {
	Season string               `json:"season"`
	Places []petSeasonPlaceJSON `json:"places"`
}

type petQuestJSON struct {
	Kind     string `json:"kind"`
	Goal     int    `json:"goal"`
	Progress int    `json:"progress"`
}

type petAchievementJSON struct {
	ID    string `json:"id"`
	GotAt string `json:"gotAt"`
}

type petListingJSON struct {
	User *authorJSON `json:"user"`
	Pet  petJSON     `json:"pet"`
}

// arenaPeople are the people of the arena.
func (a *api) arenaPeople(r *http.Request, arena model.PetArena) (map[string]model.Author, error) {
	ids := []string{arena.Boss.KilledBy}
	for _, h := range arena.Boss.Hits {
		ids = append(ids, h.UserID)
	}
	for _, t := range []*model.PetTournament{&arena.Tournament, arena.Last} {
		if t == nil {
			continue
		}
		for _, e := range t.Entrants {
			ids = append(ids, e.OwnerID)
		}
	}
	places := arena.Top
	for _, s := range arena.Seasons {
		places = append(places, s.Places...)
	}
	for _, p := range places {
		ids = append(ids, p.OwnerID)
	}
	for _, l := range arena.Market {
		ids = append(ids, l.OwnerID)
	}

	return a.people(r, ids)
}

// petOwner is the author id among people, nil when their page is gone.
func petOwner(id string, people map[string]model.Author) *authorJSON {
	p, ok := people[id]
	if !ok || p.Deactivated {
		return nil
	}

	j := toAuthorJSON(p)

	return &j
}

func toPetArenaJSON(a model.PetArena, people map[string]model.Author) petArenaJSON {
	out := petArenaJSON{
		Boss:         toPetBossJSON(a.Boss, people),
		Tournament:   toPetTournamentJSON(a.Tournament, people),
		SeasonEndsAt: timeString(a.SeasonEndsAt),
		Top:          toPetSeasonPlacesJSON(a.Top, people),
		Seasons:      make([]petSeasonJSON, 0, len(a.Seasons)),
		Quests:       make([]petQuestJSON, 0, len(a.Quests)),
		Streak:       a.Streak,
		Achievements: make([]petAchievementJSON, 0, len(a.Achievements)),
		Market:       make([]petListingJSON, 0, len(a.Market)),
	}
	if a.Last != nil {
		last := toPetTournamentJSON(*a.Last, people)
		out.Last = &last
	}
	for _, s := range a.Seasons {
		out.Seasons = append(out.Seasons, petSeasonJSON{Season: s.Season.Format(dateLayout), Places: toPetSeasonPlacesJSON(s.Places, people)})
	}
	for _, q := range a.Quests {
		out.Quests = append(out.Quests, petQuestJSON(q))
	}
	for _, g := range a.Achievements {
		out.Achievements = append(out.Achievements, petAchievementJSON{ID: g.ID, GotAt: timeString(g.GotAt)})
	}
	for _, l := range a.Market {
		out.Market = append(out.Market, petListingJSON{User: petOwner(l.OwnerID, people), Pet: toPetJSON(l.Pet)})
	}

	return out
}

func toPetBossJSON(b model.PetBoss, people map[string]model.Author) petBossJSON {
	out := petBossJSON{
		Kind: b.Kind, Health: b.Health, MaxHealth: b.MaxHealth, KilledBy: petOwner(b.KilledBy, people), KilledAt: optionalTime(b.KilledAt),
		Hits: make([]petBossHitJSON, 0, len(b.Hits)), Stats: petPowerJSON(b.Stats), Prize: b.Prize, LastHit: b.LastHit,
	}
	for _, h := range b.Hits {
		out.Hits = append(out.Hits, petBossHitJSON{User: petOwner(h.UserID, people), Damage: h.Damage, Hits: h.Hits})
	}

	return out
}

func toPetTournamentJSON(t model.PetTournament, people map[string]model.Author) petTournamentJSON {
	out := petTournamentJSON{
		StartsAt: timeString(t.StartsAt), Held: t.Held, Entrants: make([]petEntrantJSON, 0, len(t.Entrants)),
		Rounds: make([][]petMatchJSON, 0, len(t.Rounds)), Places: t.Places, Bets: t.Bets, Pool: t.Pool,
		BetPetID: t.BetPetID, BetVotes: t.BetVotes,
	}
	if out.Places == nil {
		out.Places = []int{}
	}
	for _, e := range t.Entrants {
		out.Entrants = append(out.Entrants, petEntrantJSON{
			User: petOwner(e.OwnerID, people), PetID: e.PetID, Kind: e.Kind, Name: e.Name, Level: e.Level, Power: e.Power,
		})
	}
	for _, r := range t.Rounds {
		round := make([]petMatchJSON, 0, len(r))
		for _, m := range r {
			match := petMatchJSON{A: m.A, B: m.B, Winner: m.Winner}
			if m.Bout != nil {
				bout := toPetFightJSON(*m.Bout, people)
				match.Bout = &bout
			}
			round = append(round, match)
		}
		out.Rounds = append(out.Rounds, round)
	}

	return out
}

func toPetSeasonPlacesJSON(places []model.PetSeasonPlace, people map[string]model.Author) []petSeasonPlaceJSON {
	out := make([]petSeasonPlaceJSON, 0, len(places))
	for _, p := range places {
		out = append(out, petSeasonPlaceJSON{
			User: petOwner(p.OwnerID, people), PetID: p.PetID, Kind: p.Kind, Name: p.Name, Rating: p.Rating, Prize: p.Prize,
		})
	}

	return out
}

// getPetArena is the arena of the pets as the caller sees it.
func (a *api) getPetArena(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	arena, err := a.pets.Arena(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.arenaPeople(r, arena)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toPetArenaJSON(arena, people))
}

// petArenaRequest is an action on the arena: "boss", "join", "leave",
// "bet", "sell", "unsell" or "buy"; votes are the stake of a bet or the
// price of a sale.
type petArenaRequest struct {
	Action string `json:"action"`
	PetID  string `json:"petId"`
	Votes  int64  `json:"votes"`
}

// runPetArena does an action on the arena and answers with the arena
// after it, the hit of the boss and the votes after a payment.
func (a *api) runPetArena(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req petArenaRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	done, err := a.pets.RunArena(r.Context(), token, model.PetArenaAction(req))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.arenaPeople(r, done.Arena)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var bout *petFightJSON
	if done.Bout != nil {
		b := toPetFightJSON(*done.Bout, people)
		bout = &b
	}

	writeJSON(w, http.StatusOK, struct {
		Arena   petArenaJSON  `json:"arena"`
		Bout    *petFightJSON `json:"bout"`
		Balance int64         `json:"balance"`
	}{toPetArenaJSON(done.Arena, people), bout, done.Balance})
}
