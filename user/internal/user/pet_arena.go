package user

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

// The arena of the pets: what they do together besides fighting one
// another — a boss of the week all of them hit, a tournament on Sunday
// with bets of the onlookers, and seasons of the rating, a month each,
// whose best are rewarded. The weeks and the months are of Moscow, as the
// days of the bonus.

const (
	// PetBossHealth is the health of the boss of a week.
	PetBossHealth = 20_000
	// PetBossPrize are the votes the boss brings those who beat it, by
	// the damage each dealt, PetBossLastHit more to the one who finished
	// it.
	PetBossPrize   = 3_000
	PetBossLastHit = 300
	// PetBossRest is how long a pet rests after a hit of the boss.
	PetBossRest = 2 * time.Hour
	// PetBossShards are the shards a hit brings.
	PetBossShards = 1

	// PetTournamentSize are the pets of a tournament; PetTournamentMin the
	// fewest it is held with.
	PetTournamentSize = 16
	PetTournamentMin  = 2
	// PetBetCut is the percent of the bets the treasury takes.
	PetBetCut = 10
	// MinPetBet and MaxPetBet bound a bet.
	MinPetBet = 10
	MaxPetBet = 100_000

	// PetSeasonKeeps is the part of its distance from the start a rating
	// keeps into a new season, in percent.
	PetSeasonKeeps = 50
)

// PetTournamentPrizes are the votes of the places of a tournament: the
// winner, the finalist and the two semifinalists.
var PetTournamentPrizes = []int64{1_000, 400, 150, 150}

// PetSeasonPrizes are the votes of the best of a season, the first first;
// PetSeasonTop of them are remembered.
var PetSeasonPrizes = []int64{2_000, 1_000, 500, 200, 200, 200, 200, 200, 200, 200}

const PetSeasonTop = 10

var (
	ErrPetBossDown         = errors.New("the boss of the week is beaten")
	ErrPetBossRest         = errors.New("the pet rests after its hit of the boss")
	ErrPetTournamentClosed = errors.New("the tournament of the week has begun")
	ErrPetTournamentFull   = errors.New("the tournament is full")
	ErrPetTournamentJoined = errors.New("you have a pet in the tournament already")
	ErrPetNotInTournament  = errors.New("the pet is not in the tournament")
	ErrPetBet              = errors.New("a bet is from 10 to 100000 votes")
	ErrPetBetPlaced        = errors.New("you have bet on the tournament already")
)

// PetWeek is the Monday of the week of now in Moscow, as a date.
func PetWeek(now time.Time) time.Time {
	day := BonusDay(now)

	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// PetSeason is the first day of the month of now in Moscow, as a date.
func PetSeason(now time.Time) time.Time {
	day := BonusDay(now)

	return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// PetSeasonEnd is when the season of now ends.
func PetSeasonEnd(now time.Time) time.Time {
	next := PetSeason(now).AddDate(0, 1, 0)

	return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, moscow)
}

// SeasonRating is a rating carried into a new season: half as far from
// the start.
func SeasonRating(rating int) int {
	return PetStartRating + (rating-PetStartRating)*PetSeasonKeeps/100
}

// Exhibition is a fight of a against b by their stances in which nothing
// is at stake: neither tires, eats or changes rating. The tournament is
// fought so.
func Exhibition(a, b *Pet, dice PetDice) PetBout {
	as, bs := a.Stats(), b.Stats()
	battle := &PetBattle{s: PetBattleState{
		PetID: a.s.ID, DefenderPet: b.s.ID,
		Attacker: a.combatant(a.s.Stance, as, 0), Defender: b.combatant(b.s.Stance, bs, 0), Ratio: 100,
	}}
	battle, _ = battle.Moved(PetAuto, dice) // a battle just begun is not over

	return battle.Bout(0)
}

// The boss.

// petBosses are the kinds the boss takes, a week after another.
var petBosses = []PetKind{PetDragon, PetWolf, PetPanda}

// petBossStats are the stats of the boss but its health.
var petBossStats = PetStats{Attack: 14, Defense: 8, Speed: 6}

// PetBossHit is the damage a person dealt to the boss of a week in hits.
type PetBossHit struct {
	UserID uuid.UUID
	Damage int
	Hits   int
}

// PetBossState is the boss of a week: its kind, the health left, who
// finished it and when, and the hits it took.
type PetBossState struct {
	Week     time.Time
	Kind     PetKind
	Health   int
	KilledBy uuid.UUID
	KilledAt time.Time
	Hits     []PetBossHit
}

// PetBoss is the boss of a week all the pets hit together.
type PetBoss struct {
	s PetBossState
}

// NewPetBoss is the boss of the week of now, whole.
func NewPetBoss(now time.Time) *PetBoss {
	week := PetWeek(now)
	n := int(week.Unix()/(7*24*3600)) % len(petBosses)

	return &PetBoss{s: PetBossState{Week: week, Kind: petBosses[n], Health: PetBossHealth}}
}

func RestorePetBoss(s PetBossState) *PetBoss {
	return &PetBoss{s: s}
}

// State is what storage keeps.
func (b *PetBoss) State() PetBossState {
	s := b.s
	s.Hits = slices.Clone(s.Hits)

	return s
}

func (b *PetBoss) Week() time.Time     { return b.s.Week }
func (b *PetBoss) Kind() PetKind       { return b.s.Kind }
func (b *PetBoss) Health() int         { return b.s.Health }
func (b *PetBoss) KilledBy() uuid.UUID { return b.s.KilledBy }
func (b *PetBoss) KilledAt() time.Time { return b.s.KilledAt }
func (b *PetBoss) Hits() []PetBossHit  { return slices.Clone(b.s.Hits) }
func (b *PetBoss) Down() bool          { return b.s.Health == 0 }
func (b *PetBoss) Stats() PetStats     { return petBossStats.Add(PetStats{Health: PetBossHealth}) }
func (b *PetBoss) HitOf(userID uuid.UUID) PetBossHit {
	for _, h := range b.s.Hits {
		if h.UserID == userID {
			return h
		}
	}

	return PetBossHit{UserID: userID}
}

// BossHitAt is when the pet may hit the boss again.
func (p *Pet) BossHitAt() time.Time {
	if p.s.BossAt.IsZero() {
		return time.Time{}
	}

	return p.s.BossAt.Add(PetBossRest)
}

// Hit is the pet of owner hitting the boss at now, as it fights in its
// stance, tired by its fights in a row: the pet and the boss after it and
// the fight. The pet takes no harm but a rest of PetBossRest; it gets
// what a fight gives in experience. ErrPetBossDown, ErrPetBossRest, and
// a pet that may not fight does not hit either.
func (b *PetBoss) Hit(owner uuid.UUID, p *Pet, shop PetShop, now time.Time, dice PetDice) (*PetBoss, *Pet, PetBout, error) {
	switch {
	case b.Down():
		return nil, nil, PetBout{}, ErrPetBossDown
	case now.Before(p.BossHitAt()):
		return nil, nil, PetBout{}, ErrPetBossRest
	case p.SatietyAt(now) < PetFightNeeds:
		return nil, nil, PetBout{}, ErrPetHungry
	case p.JoyAt(now) < PetFightNeeds:
		return nil, nil, PetBout{}, ErrPetSad
	case p.s.SalePrice > 0:
		return nil, nil, PetBout{}, ErrPetOnSale
	}

	boss := petBossStats.Add(PetStats{Health: b.s.Health})
	bossPet := &Pet{s: PetState{Kind: b.s.Kind, Name: PetName(b.s.Kind), Stance: PetAttack}}
	as := p.FightStatsAt(now)
	battle := &PetBattle{s: PetBattleState{
		PetID: p.s.ID, Attacker: p.combatant(p.s.Stance, as, p.FatigueAt(now)), Defender: bossPet.combatant(PetAttack, boss, 0), Ratio: 100,
	}}
	battle, _ = battle.Moved(PetAuto, dice) // a battle just begun is not over
	bout := battle.Bout(0)
	damage := b.s.Health - battle.s.Defender.Health

	n := b.State()
	n.Health -= damage
	i := slices.IndexFunc(n.Hits, func(h PetBossHit) bool { return h.UserID == owner })
	if i < 0 {
		n.Hits = append(n.Hits, PetBossHit{UserID: owner})
		i = len(n.Hits) - 1
	}
	n.Hits[i].Damage += damage
	n.Hits[i].Hits++
	if n.Health == 0 {
		n.KilledBy, n.KilledAt = owner, now
	}

	s := p.counted(now)
	s.BossAt = now
	s.Experience += PetWinGain

	return &PetBoss{s: n}, &Pet{s: s}, bout, nil
}

// Prizes are what the beaten boss brings: PetBossPrize split by the damage
// each dealt, and PetBossLastHit to the one who finished it.
func (b *PetBoss) Prizes() []Payout {
	if !b.Down() {
		return nil
	}

	var total int64
	for _, h := range b.s.Hits {
		total += int64(h.Damage)
	}

	var out []Payout
	for _, h := range b.s.Hits {
		votes := int64(PetBossPrize) * int64(h.Damage) / max(total, 1)
		if h.UserID == b.s.KilledBy {
			votes += PetBossLastHit
		}
		if votes > 0 {
			out = append(out, Payout{UserID: h.UserID, Votes: votes})
		}
	}

	return out
}

// The tournament.

// PetEntrant is a pet in a tournament as it entered.
type PetEntrant struct {
	Owner uuid.UUID
	PetID uuid.UUID
	Kind  PetKind
	Name  PetName
	Level int
	Power int
}

// PetMatch is a fight of a tournament: the entrants by their places and
// the one who won (A or B; a pet alone in its match goes on).
type PetMatch struct {
	A, B   int
	Winner int
	Bout   *PetBout
}

// PetBet is a bet of a person on a pet of a tournament.
type PetBet struct {
	UserID uuid.UUID
	PetID  uuid.UUID
	Votes  int64
}

// PetTournamentState is the tournament of a week: the entrants, the bets,
// the rounds fought and when, empty before Sunday.
type PetTournamentState struct {
	Week     time.Time
	Entrants []PetEntrant
	Bets     []PetBet
	Rounds   [][]PetMatch
	RanAt    time.Time
}

// PetTournament is the tournament of a week: pets enter it until it
// begins on Sunday at 20:00 of Moscow, the strongest first in the bracket,
// and fight it out by their stances with nothing else at stake.
type PetTournament struct {
	s PetTournamentState
}

// NewPetTournament is the tournament of the week of now, open.
func NewPetTournament(now time.Time) *PetTournament {
	return &PetTournament{s: PetTournamentState{Week: PetWeek(now)}}
}

func RestorePetTournament(s PetTournamentState) *PetTournament {
	return &PetTournament{s: s}
}

// State is what storage keeps.
func (t *PetTournament) State() PetTournamentState {
	s := t.s
	s.Entrants, s.Bets = slices.Clone(s.Entrants), slices.Clone(s.Bets)
	s.Rounds = make([][]PetMatch, len(t.s.Rounds))
	for i, r := range t.s.Rounds {
		s.Rounds[i] = slices.Clone(r)
	}

	return s
}

func (t *PetTournament) Week() time.Time         { return t.s.Week }
func (t *PetTournament) Entrants() []PetEntrant  { return slices.Clone(t.s.Entrants) }
func (t *PetTournament) Bets() []PetBet          { return slices.Clone(t.s.Bets) }
func (t *PetTournament) Rounds() [][]PetMatch    { return t.State().Rounds }
func (t *PetTournament) RanAt() time.Time        { return t.s.RanAt }
func (t *PetTournament) Held() bool              { return !t.s.RanAt.IsZero() }
func (t *PetTournament) StartsAt() time.Time     { return PetTournamentStart(t.s.Week) }
func (t *PetTournament) Open(now time.Time) bool { return now.Before(t.StartsAt()) }
func (t *PetTournament) Due(now time.Time) bool  { return !t.Held() && !t.Open(now) }
func (t *PetTournament) BetOf(userID uuid.UUID) (PetBet, bool) {
	i := slices.IndexFunc(t.s.Bets, func(b PetBet) bool { return b.UserID == userID })
	if i < 0 {
		return PetBet{}, false
	}

	return t.s.Bets[i], true
}

// PetTournamentStart is when the tournament of week begins: on its Sunday
// at 20:00 of Moscow.
func PetTournamentStart(week time.Time) time.Time {
	sunday := week.AddDate(0, 0, 6)

	return time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 20, 0, 0, 0, moscow)
}

// Joined is the tournament with the pet p of owner in it at now.
// ErrPetTournamentClosed, ErrPetTournamentFull, ErrPetTournamentJoined.
func (t *PetTournament) Joined(owner uuid.UUID, p *Pet, now time.Time) (*PetTournament, error) {
	switch {
	case !t.Open(now):
		return nil, ErrPetTournamentClosed
	case slices.ContainsFunc(t.s.Entrants, func(e PetEntrant) bool { return e.Owner == owner }):
		return nil, ErrPetTournamentJoined
	case len(t.s.Entrants) >= PetTournamentSize:
		return nil, ErrPetTournamentFull
	}

	s := t.State()
	s.Entrants = append(s.Entrants, PetEntrant{Owner: owner, PetID: p.s.ID, Kind: p.s.Kind, Name: p.s.Name, Level: p.Level(), Power: p.Power()})

	return &PetTournament{s: s}, nil
}

// Left is the tournament without the pet of owner, at now. A pet leaves
// with its owner's bet on it left standing. ErrPetTournamentClosed,
// ErrPetNotInTournament.
func (t *PetTournament) Left(owner uuid.UUID, now time.Time) (*PetTournament, error) {
	if !t.Open(now) {
		return nil, ErrPetTournamentClosed
	}

	s := t.State()
	n := len(s.Entrants)
	s.Entrants = slices.DeleteFunc(s.Entrants, func(e PetEntrant) bool { return e.Owner == owner })
	if len(s.Entrants) == n {
		return nil, ErrPetNotInTournament
	}

	return &PetTournament{s: s}, nil
}

// Bet is the tournament with a bet of userID of votes on the pet petID at
// now: one bet a person. ErrPetTournamentClosed, ErrPetNotInTournament,
// ErrPetBet, ErrPetBetPlaced.
func (t *PetTournament) Bet(userID, petID uuid.UUID, votes int64, now time.Time) (*PetTournament, error) {
	switch {
	case !t.Open(now):
		return nil, ErrPetTournamentClosed
	case !slices.ContainsFunc(t.s.Entrants, func(e PetEntrant) bool { return e.PetID == petID }):
		return nil, ErrPetNotInTournament
	case votes < MinPetBet || votes > MaxPetBet:
		return nil, ErrPetBet
	}
	if _, ok := t.BetOf(userID); ok {
		return nil, ErrPetBetPlaced
	}

	s := t.State()
	s.Bets = append(s.Bets, PetBet{UserID: userID, PetID: petID, Votes: votes})

	return &PetTournament{s: s}, nil
}

// Played is the tournament fought at now among pets — the entrants as they
// are now, those gone left out — the strongest seeded apart, a match
// after another with dice of its own. Too few pets and it is held
// without fights: Winner is then none, and the bets go back.
func (t *PetTournament) Played(pets map[uuid.UUID]*Pet, now time.Time, dice func() PetDice) *PetTournament {
	s := t.State()
	s.RanAt = now
	s.Entrants = slices.DeleteFunc(s.Entrants, func(e PetEntrant) bool { return pets[e.PetID] == nil })
	for i, e := range s.Entrants {
		p := pets[e.PetID]
		s.Entrants[i].Level, s.Entrants[i].Power = p.Level(), p.Power()
	}
	slices.SortStableFunc(s.Entrants, func(a, b PetEntrant) int { return b.Power - a.Power })
	if len(s.Entrants) < PetTournamentMin {
		return &PetTournament{s: s}
	}

	alive := seeded(len(s.Entrants))
	for len(alive) > 1 {
		var round []PetMatch
		var next []int
		for i := 0; i < len(alive); i += 2 {
			a, b := alive[i], alive[i+1]
			if a < 0 {
				a, b = b, a
			}
			m := PetMatch{A: a, B: b, Winner: a}
			if b >= 0 {
				bout := Exhibition(pets[s.Entrants[m.A].PetID], pets[s.Entrants[m.B].PetID], dice())
				m.Bout = &bout
				if !bout.Won {
					m.Winner = m.B
				}
			}
			round = append(round, m)
			next = append(next, m.Winner)
		}
		s.Rounds = append(s.Rounds, round)
		alive = next
	}

	return &PetTournament{s: s}
}

// seeded are the places 0..n-1 laid out in a bracket of a power of two,
// the strongest apart: 0 meets the weakest, and 0 and 1 meet only in the
// final; the places past n are -1, so those against them go on at once.
func seeded(n int) []int {
	size := 1
	for size < n {
		size *= 2
	}

	order := []int{0}
	for len(order) < size {
		var next []int
		total := len(order) * 2
		for _, p := range order {
			next = append(next, p, total-1-p)
		}
		order = next
	}

	for i, p := range order {
		if p >= n {
			order[i] = -1
		}
	}

	return order
}

// Winner is the entrant who won, false before the tournament or without
// fights.
func (t *PetTournament) Winner() (PetEntrant, bool) {
	if len(t.s.Rounds) == 0 {
		return PetEntrant{}, false
	}

	final := t.s.Rounds[len(t.s.Rounds)-1]

	return t.s.Entrants[final[0].Winner], true
}

// Places are the entrants by their places: the winner, the finalist and
// those who lost the semifinals, as far as there were any.
func (t *PetTournament) Places() []PetEntrant {
	w, ok := t.Winner()
	if !ok {
		return nil
	}

	out := []PetEntrant{w}
	for back := 1; back <= 2 && back <= len(t.s.Rounds); back++ {
		for _, m := range t.s.Rounds[len(t.s.Rounds)-back] {
			if m.B < 0 {
				continue
			}
			loser := m.A
			if m.Winner == m.A {
				loser = m.B
			}
			out = append(out, t.s.Entrants[loser])
		}
	}

	return out
}

// Prizes are the votes of the places.
func (t *PetTournament) Prizes() []Payout {
	var out []Payout
	for i, e := range t.Places() {
		if i < len(PetTournamentPrizes) {
			out = append(out, Payout{UserID: e.Owner, Votes: PetTournamentPrizes[i]})
		}
	}

	return out
}

// Winnings are what the bets bring, a bet back to each who staked on a
// pet out of it, and the cut of the treasury: those who bet on the winner
// share all the stakes but PetBetCut percent by their stakes; with no
// winner, or none on it, every stake goes back whole.
func (t *PetTournament) Winnings() (paid []Payout, cut int64) {
	var pool, onWinner int64
	w, ok := t.Winner()
	for _, b := range t.s.Bets {
		pool += b.Votes
		if ok && b.PetID == w.PetID {
			onWinner += b.Votes
		}
	}

	if !ok || onWinner == 0 {
		for _, b := range t.s.Bets {
			paid = append(paid, Payout{UserID: b.UserID, Votes: b.Votes})
		}

		return paid, 0
	}

	cut = pool * PetBetCut / 100
	var shared int64
	for _, b := range t.s.Bets {
		if b.PetID != w.PetID {
			continue
		}
		votes := (pool - cut) * b.Votes / onWinner
		shared += votes
		paid = append(paid, Payout{UserID: b.UserID, Votes: votes})
	}

	return paid, pool - shared
}

// The season.

// PetSeasonPlace is a place of a season: the pet and its owner, its
// rating at the end and the votes it brought.
type PetSeasonPlace struct {
	Owner  uuid.UUID
	PetID  uuid.UUID
	Kind   PetKind
	Name   PetName
	Rating int
	Prize  int64
}

// PetSeasonPlaces are the places of a season of the best pets, the
// highest rating first: one pet an owner, the best of theirs, at most
// PetSeasonTop.
func PetSeasonPlaces(best []PetContender) []PetSeasonPlace {
	var out []PetSeasonPlace
	seen := map[uuid.UUID]bool{}
	for _, c := range best {
		if seen[c.Owner] || len(out) >= PetSeasonTop {
			continue
		}
		seen[c.Owner] = true
		out = append(out, PetSeasonPlace{
			Owner: c.Owner, PetID: c.Pet.ID(), Kind: c.Pet.Kind(), Name: c.Pet.Name(), Rating: c.Pet.Rating(), Prize: PetSeasonPrizes[len(out)],
		})
	}

	return out
}

// PetSeasonResult is a season closed: its first day and its places.
type PetSeasonResult struct {
	Season time.Time
	Places []PetSeasonPlace
}
