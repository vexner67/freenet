package inmem

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// petArena is the boss, the tournaments, the seasons, the tasks and the
// badges in memory.
type petArena struct {
	bosses       map[time.Time]user.PetBossState
	tournaments  map[time.Time]user.PetTournamentState
	seasons      []user.PetSeasonResult
	quests       map[uuid.UUID]user.PetQuestsState
	achievements map[uuid.UUID][]user.PetAchievementGot
}

func newPetArena() petArena {
	return petArena{
		bosses: map[time.Time]user.PetBossState{}, tournaments: map[time.Time]user.PetTournamentState{},
		quests: map[uuid.UUID]user.PetQuestsState{}, achievements: map[uuid.UUID][]user.PetAchievementGot{},
	}
}

func (r *PetRepository) Boss(ctx context.Context, week time.Time) (*user.PetBoss, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.arena.bosses[week]
	if !ok {
		return nil, user.ErrNotFound
	}

	return user.RestorePetBoss(s), nil
}

func (r *PetRepository) LockBoss(ctx context.Context, fresh *user.PetBoss) (*user.PetBoss, error) {
	b, err := r.Boss(ctx, fresh.Week())
	if missingWeek(err) {
		return fresh, nil
	}

	return b, err
}

func (r *PetRepository) SaveBoss(ctx context.Context, b *user.PetBoss) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.arena.bosses[b.Week()] = b.State()

	return nil
}

func (r *PetRepository) Tournament(ctx context.Context, week time.Time) (*user.PetTournament, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.arena.tournaments[week]
	if !ok {
		return nil, user.ErrNotFound
	}

	return user.RestorePetTournament(s), nil
}

func (r *PetRepository) LockTournament(ctx context.Context, fresh *user.PetTournament) (*user.PetTournament, error) {
	t, err := r.Tournament(ctx, fresh.Week())
	if missingWeek(err) {
		return fresh, nil
	}

	return t, err
}

func (r *PetRepository) SaveTournament(ctx context.Context, t *user.PetTournament) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.arena.tournaments[t.Week()] = t.State()

	return nil
}

// missingWeek is whether err says nothing was kept of a week.
func missingWeek(err error) bool {
	return errors.Is(err, user.ErrNotFound)
}

// Best are the pets of the highest rating in memory.
func (r *PetRepository) Best(ctx context.Context, now time.Time, limit int) ([]user.PetContender, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	pets := r.ranked(ctx, now)
	best := make([]user.PetContender, 0, min(len(pets), limit))
	for _, o := range pets[:min(len(pets), limit)] {
		best = append(best, user.PetContender{Owner: o.owner, Pet: user.RestorePet(o.state)})
	}

	return best, nil
}

func (r *PetRepository) Seasons(ctx context.Context, limit int) ([]user.PetSeasonResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	seasons := slices.Clone(r.arena.seasons)
	slices.Reverse(seasons)

	return seasons[:min(len(seasons), limit)], nil
}

func (r *PetRepository) CloseSeason(ctx context.Context, result user.PetSeasonResult) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if slices.ContainsFunc(r.arena.seasons, func(s user.PetSeasonResult) bool { return s.Season.Equal(result.Season) }) {
		return false, nil
	}
	r.arena.seasons = append(r.arena.seasons, user.PetSeasonResult{Season: result.Season, Places: slices.Clone(result.Places)})

	for id, o := range r.pets {
		o.state.Rating = user.SeasonRating(o.state.Rating)
		r.pets[id] = o
	}

	return true, nil
}

func (r *PetRepository) LockQuests(ctx context.Context, userID uuid.UUID) (*user.PetQuests, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return user.RestorePetQuests(userID, r.arena.quests[userID]), nil
}

func (r *PetRepository) SaveQuests(ctx context.Context, userID uuid.UUID, q *user.PetQuests) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.arena.quests[userID] = q.State()

	return nil
}

func (r *PetRepository) Achievements(ctx context.Context, userID uuid.UUID) ([]user.PetAchievementGot, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.arena.achievements[userID]), nil
}

func (r *PetRepository) Achieve(ctx context.Context, userID uuid.UUID, got []user.PetAchievement, now time.Time) error {
	if _, err := r.users.GetByID(ctx, userID); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	had := r.arena.achievements[userID]
	for _, a := range got {
		if !slices.ContainsFunc(had, func(g user.PetAchievementGot) bool { return g.Achievement == a }) {
			had = append(had, user.PetAchievementGot{Achievement: a, At: now})
		}
	}
	r.arena.achievements[userID] = had

	return nil
}

// Listings are the pets up for sale in memory.
func (r *PetRepository) Listings(ctx context.Context, now time.Time, limit int) ([]user.PetContender, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	pets := slices.DeleteFunc(r.ranked(ctx, now), func(o owned) bool { return o.state.SalePrice == 0 })
	slices.SortStableFunc(pets, func(a, b owned) int { return int(a.state.SalePrice - b.state.SalePrice) })

	listed := make([]user.PetContender, 0, min(len(pets), limit))
	for _, o := range pets[:min(len(pets), limit)] {
		listed = append(listed, user.PetContender{Owner: o.owner, Pet: user.RestorePet(o.state)})
	}

	return listed, nil
}

func (r *PetRepository) LockListed(ctx context.Context, petID uuid.UUID) (user.PetContender, error) {
	if err := ctx.Err(); err != nil {
		return user.PetContender{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	o, ok := r.pets[petID]
	if !ok {
		return user.PetContender{}, user.ErrPetNotFound
	}

	return user.PetContender{Owner: o.owner, Pet: user.RestorePet(o.state)}, nil
}

func (r *PetRepository) MovePet(ctx context.Context, petID, toID uuid.UUID) error {
	if _, err := r.users.GetByID(ctx, toID); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	o, ok := r.pets[petID]
	if !ok {
		return user.ErrPetNotFound
	}

	var kept int
	for _, p := range r.pets {
		if p.owner == toID {
			kept++
		}
	}
	if kept >= user.MaxPets {
		return user.ErrTooManyPets
	}

	o.owner = toID
	r.pets[petID] = o

	return nil
}
