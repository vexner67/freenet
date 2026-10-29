package user

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Quests of the day and achievements of the pets: three tasks a day,
// drawn for each person, each paying votes once done, all three — shards
// and a bonus growing with the days in a row; and badges for what was
// done once and for all.

// PetQuestKind is a task of the day.
type PetQuestKind string

const (
	PetQuestWin    PetQuestKind = "win"
	PetQuestFight  PetQuestKind = "fight"
	PetQuestCase   PetQuestKind = "case"
	PetQuestFeed   PetQuestKind = "feed"
	PetQuestPlay   PetQuestKind = "play"
	PetQuestBoss   PetQuestKind = "boss"
	PetQuestManual PetQuestKind = "manual"
)

// petQuestGoals are the tasks of the day and how many times each is done.
var petQuestGoals = []struct {
	kind PetQuestKind
	goal int
}{
	{PetQuestWin, 3}, {PetQuestFight, 5}, {PetQuestCase, 1}, {PetQuestFeed, 2},
	{PetQuestPlay, 2}, {PetQuestBoss, 1}, {PetQuestManual, 1},
}

const (
	// PetQuestsADay are the tasks of a day.
	PetQuestsADay = 3
	// PetQuestReward are the votes of a task done; all of the day bring
	// PetQuestShards shards and PetStreakReward votes a day in a row, up
	// to MaxPetStreakDays days.
	PetQuestReward   = 15
	PetQuestShards   = 5
	PetStreakReward  = 10
	MaxPetStreakDays = 7
)

// PetQuest is a task of the day: what to do and how many times.
type PetQuest struct {
	Kind PetQuestKind
	Goal int
}

// PetQuestsOf are the tasks of userID on day, drawn of the hash of both:
// the same all day, others the next.
func PetQuestsOf(userID uuid.UUID, day time.Time) []PetQuest {
	order := make([]PetQuest, len(petQuestGoals))
	for i, g := range petQuestGoals {
		order[i] = PetQuest{Kind: g.kind, Goal: g.goal}
	}

	key := func(q PetQuest) uint64 {
		h := sha256.New()
		h.Write(userID[:])
		h.Write([]byte(day.Format(time.DateOnly)))
		h.Write([]byte(q.Kind))

		return binary.BigEndian.Uint64(h.Sum(nil))
	}
	slices.SortFunc(order, func(a, b PetQuest) int {
		ka, kb := key(a), key(b)
		switch {
		case ka < kb:
			return -1
		case ka > kb:
			return 1
		}

		return 0
	})

	return order[:PetQuestsADay]
}

// PetQuestsState is how far a person got with the tasks of Day, and the
// days in a row they did all of them, the last StreakDay.
type PetQuestsState struct {
	Day       time.Time
	Progress  map[PetQuestKind]int
	Streak    int
	StreakDay time.Time
}

// PetQuests are the tasks of a person.
type PetQuests struct {
	userID uuid.UUID
	s      PetQuestsState
}

func RestorePetQuests(userID uuid.UUID, s PetQuestsState) *PetQuests {
	return &PetQuests{userID: userID, s: s}
}

// State is what storage keeps.
func (q *PetQuests) State() PetQuestsState {
	s := q.s
	s.Progress = make(map[PetQuestKind]int, len(q.s.Progress))
	for k, v := range q.s.Progress {
		s.Progress[k] = v
	}

	return s
}

// Today are the tasks of the day of now.
func (q *PetQuests) Today(now time.Time) []PetQuest {
	return PetQuestsOf(q.userID, BonusDay(now))
}

// ProgressAt is how many times a task was done on the day of now.
func (q *PetQuests) ProgressAt(kind PetQuestKind, now time.Time) int {
	if !q.s.Day.Equal(BonusDay(now)) {
		return 0
	}

	return q.s.Progress[kind]
}

// StreakAt are the days in a row all the tasks were done, today or until
// yesterday.
func (q *PetQuests) StreakAt(now time.Time) int {
	day := BonusDay(now)
	if q.s.StreakDay.Equal(day) || q.s.StreakDay.Equal(day.AddDate(0, 0, -1)) {
		return q.s.Streak
	}

	return 0
}

// PetQuestPrize is what doing a task brought: votes and shards, and the
// tasks of the day done by it.
type PetQuestPrize struct {
	Votes  int64
	Shards int
	Done   []PetQuestKind
}

// Did is the tasks after kind was done n times at now, and what it
// brought: the tasks it finished pay, and the last of the day pays the
// shards and the bonus of the days in a row.
func (q *PetQuests) Did(kind PetQuestKind, n int, now time.Time) (*PetQuests, PetQuestPrize) {
	day := BonusDay(now)
	s := q.State()
	if !s.Day.Equal(day) {
		s.Day, s.Progress = day, map[PetQuestKind]int{}
	}

	var prize PetQuestPrize
	today := PetQuestsOf(q.userID, day)
	i := slices.IndexFunc(today, func(t PetQuest) bool { return t.Kind == kind })
	if i < 0 || s.Progress[kind] >= today[i].Goal {
		return &PetQuests{userID: q.userID, s: s}, prize
	}

	s.Progress[kind] = min(s.Progress[kind]+n, today[i].Goal)
	if s.Progress[kind] < today[i].Goal {
		return &PetQuests{userID: q.userID, s: s}, prize
	}

	prize.Votes, prize.Done = PetQuestReward, []PetQuestKind{kind}
	for _, t := range today {
		if s.Progress[t.Kind] < t.Goal {
			return &PetQuests{userID: q.userID, s: s}, prize
		}
	}

	s.Streak = q.StreakAt(now) + 1
	s.StreakDay = day
	prize.Shards = PetQuestShards
	prize.Votes += int64(PetStreakReward * min(s.Streak, MaxPetStreakDays))

	return &PetQuests{userID: q.userID, s: s}, prize
}

// PetAchievement is a badge of what a person did once and for all.
type PetAchievement string

const (
	PetFirstWin     PetAchievement = "first_win"
	PetHundredWins  PetAchievement = "wins_100"
	PetVeteran      PetAchievement = "fights_500"
	PetCollector    PetAchievement = "collector"
	PetLegend       PetAchievement = "legend"
	PetFirstPhoenix PetAchievement = "phoenix"
	PetTopLevel     PetAchievement = "top_level"
	PetBossSlayer   PetAchievement = "boss_slayer"
	PetChampion     PetAchievement = "champion"
	PetSeasonBest   PetAchievement = "season_top"
	PetWeekStreak   PetAchievement = "week_streak"
	PetMillionaire  PetAchievement = "millionaire"
)

// PetAchievements are all the badges, in the order the page shows them.
var PetAchievements = []PetAchievement{
	PetFirstWin, PetHundredWins, PetVeteran, PetCollector, PetLegend, PetFirstPhoenix, PetTopLevel,
	PetBossSlayer, PetChampion, PetSeasonBest, PetWeekStreak, PetMillionaire,
}

// PetAchievementGot is a badge and when it was earned.
type PetAchievementGot struct {
	Achievement PetAchievement
	At          time.Time
}

const (
	petHundredWins    = 100
	petVeteranFights  = 500
	petCollectorKinds = 5
	petMillion        = 1_000_000
)

// EarnedAchievements are the badges what a person has now earns: their
// pets, their votes and the days in a row they did all the tasks. The
// boss, the tournament and the season give theirs when they happen.
func EarnedAchievements(pets []*Pet, votes int64, streak int) []PetAchievement {
	var wins, fights int
	kinds := map[PetKind]bool{}
	var legend, phoenix, top bool
	for _, p := range pets {
		wins += p.Wins()
		fights += p.Wins() + p.Losses()
		kinds[p.Kind()] = true
		legend = legend || p.Kind().Rarity() == PetLegendary
		phoenix = phoenix || p.Kind() == PetPhoenix
		top = top || p.Level() >= MaxPetLevel
	}

	var out []PetAchievement
	add := func(a PetAchievement, ok bool) {
		if ok {
			out = append(out, a)
		}
	}
	add(PetFirstWin, wins >= 1)
	add(PetHundredWins, wins >= petHundredWins)
	add(PetVeteran, fights >= petVeteranFights)
	add(PetCollector, len(kinds) >= petCollectorKinds)
	add(PetLegend, legend)
	add(PetFirstPhoenix, phoenix)
	add(PetTopLevel, top)
	add(PetWeekStreak, streak >= MaxPetStreakDays)
	add(PetMillionaire, votes >= petMillion)

	return out
}
