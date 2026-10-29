package grpc

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *UserHandler) GetPetArena(ctx context.Context, _ *userv1.GetPetArenaRequest) (*userv1.PetArena, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	return h.petArena(ctx, userID, time.Now())
}

func (h *UserHandler) petArena(ctx context.Context, userID uuid.UUID, now time.Time) (*userv1.PetArena, error) {
	a, err := h.pets.Arena(ctx, userID, now)
	if err != nil {
		return nil, err
	}

	return toProtoPetArena(a, userID, now), nil
}

func (h *UserHandler) RunPetArena(ctx context.Context, req *userv1.RunPetArenaRequest) (*userv1.RunPetArenaResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	act := service.PetArenaAct{Action: service.PetArenaAction(req.GetAction()), UserID: userID, Votes: req.GetVotes(), Now: time.Now()}
	if act.Action != service.PetLeave {
		if act.PetID, err = petOf(req.GetPetId()); err != nil {
			return nil, err
		}
	}

	done, err := h.pets.RunArena(ctx, act)
	if err != nil {
		return nil, err
	}

	resp := &userv1.RunPetArenaResponse{Balance: done.Balance}
	if done.Bout != nil {
		resp.Bout = toProtoPetFight(user.PetFight{AttackerID: userID, Bout: *done.Bout, At: act.Now})
	}
	if resp.Arena, err = h.petArena(ctx, userID, act.Now); err != nil {
		return nil, err
	}

	return resp, nil
}

//nolint:gosec // the numbers of the arena are small: health, levels, places and days
func toProtoPetArena(a service.PetArena, userID uuid.UUID, now time.Time) *userv1.PetArena {
	out := &userv1.PetArena{
		Boss:         toProtoPetBoss(a.Boss),
		Tournament:   toProtoPetTournament(a.Tournament, userID),
		Last:         toProtoPetTournament(a.Last, userID),
		SeasonEndsAt: timestamppb.New(user.PetSeasonEnd(now)),
		Top:          toProtoPetSeasonPlaces(a.Top),
		Streak:       int32(a.Quests.StreakAt(now)),
	}

	for _, s := range a.Seasons {
		out.Seasons = append(out.Seasons, &userv1.PetSeason{Season: timestamppb.New(s.Season), Places: toProtoPetSeasonPlaces(s.Places)})
	}
	for _, q := range a.Quests.Today(now) {
		out.Quests = append(out.Quests, &userv1.PetQuest{Kind: string(q.Kind), Goal: int32(q.Goal), Progress: int32(a.Quests.ProgressAt(q.Kind, now))})
	}
	for _, g := range a.Achievements {
		out.Achievements = append(out.Achievements, &userv1.PetAchievement{Id: string(g.Achievement), GotAt: timestamppb.New(g.At)})
	}
	for _, l := range a.Market {
		out.Market = append(out.Market, &userv1.PetListing{OwnerId: l.Owner.String(), Pet: toProtoPet(l.Pet, now)})
	}

	return out
}

//nolint:gosec // the health and the damage of the boss are small
func toProtoPetBoss(b *user.PetBoss) *userv1.PetBoss {
	hits := b.Hits()
	slices.SortStableFunc(hits, func(x, y user.PetBossHit) int { return y.Damage - x.Damage })

	out := &userv1.PetBoss{
		Week: timestamppb.New(b.Week()), Kind: b.Kind().String(), Health: int32(b.Health()), MaxHealth: user.PetBossHealth,
		Stats: toProtoPetStats(b.Stats()), Prize: user.PetBossPrize, LastHit: user.PetBossLastHit,
	}
	if b.Down() {
		out.KilledBy, out.KilledAt = b.KilledBy().String(), timestamppb.New(b.KilledAt())
	}
	for _, h := range hits {
		out.Hits = append(out.Hits, &userv1.PetBossHit{UserId: h.UserID.String(), Damage: int32(h.Damage), Hits: int32(h.Hits)})
	}

	return out
}

//nolint:gosec // the indexes and levels of a tournament are small
func toProtoPetTournament(t *user.PetTournament, userID uuid.UUID) *userv1.PetTournament {
	if t == nil {
		return nil
	}

	out := &userv1.PetTournament{Week: timestamppb.New(t.Week()), StartsAt: timestamppb.New(t.StartsAt()), Held: t.Held()}
	entrants := t.Entrants()
	for _, e := range entrants {
		out.Entrants = append(out.Entrants, &userv1.PetEntrant{
			OwnerId: e.Owner.String(), PetId: e.PetID.String(), Kind: e.Kind.String(), Name: e.Name.String(), Level: int32(e.Level), Power: int32(e.Power),
		})
	}
	for _, round := range t.Rounds() {
		r := &userv1.PetRound{}
		for _, m := range round {
			match := &userv1.PetMatch{A: int32(m.A), B: int32(m.B), Winner: int32(m.Winner)}
			if m.Bout != nil {
				match.Bout = toProtoPetFight(user.PetFight{AttackerID: entrants[m.A].Owner, DefenderID: entrants[m.B].Owner, Bout: *m.Bout, At: t.RanAt()})
			}
			r.Matches = append(r.Matches, match)
		}
		out.Rounds = append(out.Rounds, r)
	}
	for _, p := range t.Places() {
		out.Places = append(out.Places, int32(slices.IndexFunc(entrants, func(e user.PetEntrant) bool { return e.PetID == p.PetID })))
	}
	for _, b := range t.Bets() {
		out.Bets++
		out.Pool += b.Votes
	}
	if b, ok := t.BetOf(userID); ok {
		out.BetPetId, out.BetVotes = b.PetID.String(), b.Votes
	}

	return out
}

//nolint:gosec // a rating is small
func toProtoPetSeasonPlaces(places []user.PetSeasonPlace) []*userv1.PetSeasonPlace {
	out := make([]*userv1.PetSeasonPlace, len(places))
	for i, p := range places {
		out[i] = &userv1.PetSeasonPlace{
			OwnerId: p.Owner.String(), PetId: p.PetID.String(), Kind: p.Kind.String(), Name: p.Name.String(), Rating: int32(p.Rating), Prize: p.Prize,
		}
	}

	return out
}
