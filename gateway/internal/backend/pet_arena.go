package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

func (p *Pets) Arena(ctx context.Context, accessToken string) (model.PetArena, error) {
	resp, err := p.api.GetPetArena(withToken(ctx, accessToken), &userv1.GetPetArenaRequest{})
	if err != nil {
		return model.PetArena{}, errs.Wrap("p.api.GetPetArena", fromStatus(err))
	}

	return fromProtoPetArena(resp), nil
}

func (p *Pets) RunArena(ctx context.Context, accessToken string, a model.PetArenaAction) (model.PetArenaDone, error) {
	resp, err := p.api.RunPetArena(withToken(ctx, accessToken), &userv1.RunPetArenaRequest{Action: a.Action, PetId: a.PetID, Votes: a.Votes})
	if err != nil {
		return model.PetArenaDone{}, errs.Wrap("p.api.RunPetArena", fromStatus(err))
	}

	done := model.PetArenaDone{Arena: fromProtoPetArena(resp.GetArena()), Balance: resp.GetBalance()}
	if resp.GetBout() != nil {
		bout := fromProtoPetFight(resp.GetBout())
		done.Bout = &bout
	}

	return done, nil
}

func fromProtoPetArena(a *userv1.PetArena) model.PetArena {
	out := model.PetArena{
		Boss:         fromProtoPetBoss(a.GetBoss()),
		Tournament:   fromProtoPetTournament(a.GetTournament()),
		SeasonEndsAt: fromTimestamp(a.GetSeasonEndsAt()),
		Top:          fromProtoPetSeasonPlaces(a.GetTop()),
		Streak:       int(a.GetStreak()),
	}
	if a.GetLast() != nil {
		last := fromProtoPetTournament(a.GetLast())
		out.Last = &last
	}
	for _, s := range a.GetSeasons() {
		out.Seasons = append(out.Seasons, model.PetSeason{Season: fromTimestamp(s.GetSeason()), Places: fromProtoPetSeasonPlaces(s.GetPlaces())})
	}
	for _, q := range a.GetQuests() {
		out.Quests = append(out.Quests, model.PetQuest{Kind: q.GetKind(), Goal: int(q.GetGoal()), Progress: int(q.GetProgress())})
	}
	for _, g := range a.GetAchievements() {
		out.Achievements = append(out.Achievements, model.PetAchievement{ID: g.GetId(), GotAt: fromTimestamp(g.GetGotAt())})
	}
	for _, l := range a.GetMarket() {
		out.Market = append(out.Market, model.PetListing{OwnerID: l.GetOwnerId(), Pet: fromProtoPet(l.GetPet())})
	}

	return out
}

func fromProtoPetBoss(b *userv1.PetBoss) model.PetBoss {
	out := model.PetBoss{
		Kind: b.GetKind(), Health: int(b.GetHealth()), MaxHealth: int(b.GetMaxHealth()), KilledBy: b.GetKilledBy(),
		KilledAt: fromTimestamp(b.GetKilledAt()), Stats: fromProtoPetPower(b.GetStats()), Prize: b.GetPrize(), LastHit: b.GetLastHit(),
	}
	for _, h := range b.GetHits() {
		out.Hits = append(out.Hits, model.PetBossHit{UserID: h.GetUserId(), Damage: int(h.GetDamage()), Hits: int(h.GetHits())})
	}

	return out
}

func fromProtoPetTournament(t *userv1.PetTournament) model.PetTournament {
	out := model.PetTournament{
		Week: fromTimestamp(t.GetWeek()), StartsAt: fromTimestamp(t.GetStartsAt()), Held: t.GetHeld(),
		Bets: int(t.GetBets()), Pool: t.GetPool(), BetPetID: t.GetBetPetId(), BetVotes: t.GetBetVotes(),
	}
	for _, e := range t.GetEntrants() {
		out.Entrants = append(out.Entrants, model.PetEntrant{
			OwnerID: e.GetOwnerId(), PetID: e.GetPetId(), Kind: e.GetKind(), Name: e.GetName(), Level: int(e.GetLevel()), Power: int(e.GetPower()),
		})
	}
	for _, r := range t.GetRounds() {
		round := make([]model.PetMatch, 0, len(r.GetMatches()))
		for _, m := range r.GetMatches() {
			match := model.PetMatch{A: int(m.GetA()), B: int(m.GetB()), Winner: int(m.GetWinner())}
			if m.GetBout() != nil {
				bout := fromProtoPetFight(m.GetBout())
				match.Bout = &bout
			}
			round = append(round, match)
		}
		out.Rounds = append(out.Rounds, round)
	}
	for _, p := range t.GetPlaces() {
		out.Places = append(out.Places, int(p))
	}

	return out
}

func fromProtoPetSeasonPlaces(places []*userv1.PetSeasonPlace) []model.PetSeasonPlace {
	out := make([]model.PetSeasonPlace, 0, len(places))
	for _, p := range places {
		out = append(out, model.PetSeasonPlace{
			OwnerID: p.GetOwnerId(), PetID: p.GetPetId(), Kind: p.GetKind(), Name: p.GetName(), Rating: int(p.GetRating()), Prize: p.GetPrize(),
		})
	}

	return out
}
