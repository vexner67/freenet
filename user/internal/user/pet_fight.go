package user

import (
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
)

// PetStance is how a pet fights: the attacker picks it before a fight, the
// defender fights in the one its owner set in advance. Each beats another,
// as rock, paper and scissors: attack beats agility, agility beats
// defense, defense beats attack.
type PetStance string

const (
	// PetAttack hits harder.
	PetAttack PetStance = "attack"
	// PetDefense is hit softer.
	PetDefense PetStance = "defense"
	// PetAgility dodges more.
	PetAgility PetStance = "agility"
)

// PetStances are all stances in the order the site shows them.
var PetStances = []PetStance{PetAttack, PetDefense, PetAgility}

func NewPetStance(s string) (PetStance, error) {
	for _, st := range PetStances {
		if string(st) == s {
			return st, nil
		}
	}

	return "", ErrUnknownPetStance
}

func (s PetStance) String() string {
	return string(s)
}

// Beats tells whether a pet in s hits harder one in o.
func (s PetStance) Beats(o PetStance) bool {
	switch s {
	case PetAttack:
		return o == PetAgility
	case PetAgility:
		return o == PetDefense
	case PetDefense:
		return o == PetAttack
	}

	return false
}

// PetElement is what a kind is of: water beats fire, fire beats forest,
// forest beats water.
type PetElement string

const (
	PetFire   PetElement = "fire"
	PetWater  PetElement = "water"
	PetForest PetElement = "forest"
)

// PetElements are all elements.
var PetElements = []PetElement{PetFire, PetWater, PetForest}

func (e PetElement) String() string {
	return string(e)
}

// Beats tells whether a pet of e hits harder one of o.
func (e PetElement) Beats(o PetElement) bool {
	switch e {
	case PetWater:
		return o == PetFire
	case PetFire:
		return o == PetForest
	case PetForest:
		return o == PetWater
	}

	return false
}

// PetSpecial is the trick of a kind in a fight.
type PetSpecial string

const (
	// PetCounter: the cat strikes back at half strength when it dodges.
	PetCounter PetSpecial = "counter"
	// PetRage: the dog hits harder when it is nearly beaten.
	PetRage PetSpecial = "rage"
	// PetCheeks: the hamster, nearly beaten, eats instead of a hit once
	// and heals a quarter of its health.
	PetCheeks PetSpecial = "cheeks"
	// PetLeap: the rabbit hits twice at its first hit.
	PetLeap PetSpecial = "leap"
	// PetShell: the turtle, under half of its health, hides from a hit
	// once.
	PetShell PetSpecial = "shell"
	// PetTrick: the fox is never hit by the first hit.
	PetTrick PetSpecial = "trick"
	// PetSight: the owl dodges more in the second half of a fight.
	PetSight PetSpecial = "sight"
	// PetFlame: each third hit of the dragon goes through defense.
	PetFlame PetSpecial = "flame"
	// PetHowl: each hit of the wolf is harder than the one before.
	PetHowl PetSpecial = "howl"
	// PetHide: the panda takes less of each hit.
	PetHide PetSpecial = "hide"
	// PetRainbow: the unicorn heals half of a crit it deals.
	PetRainbow PetSpecial = "rainbow"
	// PetRebirth: the phoenix rises once with a quarter of its health.
	PetRebirth PetSpecial = "rebirth"
)

func (s PetSpecial) String() string {
	return string(s)
}

// PetMove is what the attacker does in a round of a fight it leads.
type PetMove string

const (
	PetStrike PetMove = "strike"
	// PetGuard heals a little and halves the next hit taken, and the next
	// hit dealt is harder.
	PetGuard PetMove = "guard"
	// PetPower hits harder, but is dodged more.
	PetPower PetMove = "power"
	// PetAuto lets the fight end by itself: both pets move by their
	// stances.
	PetAuto PetMove = "auto"
)

func NewPetMove(s string) (PetMove, error) {
	for _, m := range []PetMove{PetStrike, PetGuard, PetPower, PetAuto} {
		if string(m) == s {
			return m, nil
		}
	}

	return "", ErrUnknownPetMove
}

func (m PetMove) String() string {
	return string(m)
}

// The rules of a fight, the multipliers in percent.
const (
	// MaxPetRounds are the rounds of a fight: each pet acts once a round.
	MaxPetRounds = 8
	// PetDiceCount is the random numbers a fight may take.
	PetDiceCount = MaxPetRounds * 24
	// PetRest is how long a pet rests from a fight; fights closer than it
	// are in a row and tire it.
	PetRest        = time.Hour
	PetFatigueStep = 5
	MaxPetFatigue  = 6
	MaxPetDodge    = 60

	petAttackHits    = 120
	petDefenseTakes  = 80
	petAgilityDodges = 8
	petBeats         = 125
	petPowerHits     = 160
	petPowerDodged   = 15
	petGuardTakes    = 50
	petGuardCharge   = 150
	petGuardHeals    = 5
	petRageHits      = 130
	petHowlStep      = 10
	petHowlSteps     = 3
	petSightDodges   = 10
	petHideTakes     = 2
	// petRatioMin and petRatioMax bound how far the power of the defender
	// to the attacker's moves the rewards.
	petRatioMin = 25
	petRatioMax = 200
)

var (
	ErrUnknownPetStance = errors.New("unknown stance")
	ErrUnknownPetMove   = errors.New("unknown move")
	ErrPetBattleOver    = errors.New("the fight is over")
	ErrNoPetBattle      = errors.New("the pet is not in a fight")
	ErrPetInBattle      = errors.New("the pet is in a fight")
)

// PetDice are the random numbers a fight takes, each in [0, PetDiceRange);
// a fight that runs out of them reads zeros.
type PetDice []int64

// next is the next number of the dice in [0, n).
func (d *PetDice) next(n int) int {
	if len(*d) == 0 || n <= 0 {
		return 0
	}

	v := (*d)[0]
	*d = (*d)[1:]

	return int(v % int64(n))
}

// PetHit is an act of a fight: who acted and how, whether the other
// dodged or it was twice as hard, the damage, what the actor healed, its
// special and the special of the other that answered, and the health
// both are left with.
type PetHit struct {
	ByAttacker     bool
	Move           PetMove
	Dodged         bool
	Crit           bool
	Damage         int
	Heal           int
	Special        PetSpecial
	Answer         PetSpecial
	AttackerHealth int
	DefenderHealth int
}

// PetFighter is a pet as a fight remembers it: in the clothes and stance
// it fought, its power, how tired it was and its full health.
type PetFighter struct {
	Kind    PetKind
	Name    PetName
	Level   int
	Health  int
	Outfit  []PetItem
	Stance  PetStance
	Power   int
	Fatigue int
}

// PetBout is a fight: who fought, how it went and what it changed.
type PetBout struct {
	Attacker PetFighter
	Defender PetFighter
	Hits     []PetHit
	Won      bool
	// Rating is the points the attacker won (above zero) or lost; the
	// defender changes by as much the other way.
	Rating int
	// Ratio is the power of the defender to the attacker's in percent,
	// from petRatioMin to petRatioMax: the rewards of a win follow it.
	Ratio int
	// Reward is the votes the win brought.
	Reward int64
}

// PetCombatant is a pet in a fight: its stats, what is left of its
// health and what it did.
type PetCombatant struct {
	Fighter PetFighter
	Stats   PetStats
	Health  int
	Strikes int
	// Used are the specials it spent: those that work once.
	Used []PetSpecial
	// Guarding halves the next hit it takes, Charged makes the next it
	// deals harder.
	Guarding bool
	Charged  bool
}

// PetBattleState is a fight: the pet that leads it and the pet it fights,
// the combatants, the acts so far and the round, and what it brings. The
// fight is counted as lost at its start: Win is the rating a win brings
// back above Loss, WinGain the experience above PetLossGain.
type PetBattleState struct {
	PetID         uuid.UUID
	DefenderOwner uuid.UUID
	DefenderPet   uuid.UUID
	Attacker      PetCombatant
	Defender      PetCombatant
	Hits          []PetHit
	Round         int
	Ratio         int
	Win           int
	Loss          int
	WinGain       int64
	StartedAt     time.Time
}

// PetBattle is a fight of pets a round after another: the owner of the
// attacker moves it, the defender moves by its stance.
type PetBattle struct {
	s PetBattleState
}

func RestorePetBattle(s PetBattleState) *PetBattle {
	return &PetBattle{s: s}
}

// State is what storage keeps.
func (b *PetBattle) State() PetBattleState {
	s := b.s
	s.Hits = slices.Clone(s.Hits)
	s.Attacker.Used, s.Defender.Used = slices.Clone(s.Attacker.Used), slices.Clone(s.Defender.Used)
	s.Attacker.Fighter.Outfit = slices.Clone(s.Attacker.Fighter.Outfit)
	s.Defender.Fighter.Outfit = slices.Clone(s.Defender.Fighter.Outfit)

	return s
}

func (b *PetBattle) PetID() uuid.UUID         { return b.s.PetID }
func (b *PetBattle) DefenderOwner() uuid.UUID { return b.s.DefenderOwner }
func (b *PetBattle) DefenderPet() uuid.UUID   { return b.s.DefenderPet }
func (b *PetBattle) Attacker() PetCombatant   { return b.State().Attacker }
func (b *PetBattle) Defender() PetCombatant   { return b.State().Defender }
func (b *PetBattle) Hits() []PetHit           { return slices.Clone(b.s.Hits) }
func (b *PetBattle) Round() int               { return b.s.Round }
func (b *PetBattle) Ratio() int               { return b.s.Ratio }
func (b *PetBattle) StartedAt() time.Time     { return b.s.StartedAt }
func (b *PetBattle) Rating() (win, loss int)  { return b.s.Win, b.s.Loss }

// Done tells whether a pet fell or the rounds are over.
func (b *PetBattle) Done() bool {
	return b.s.Attacker.Health == 0 || b.s.Defender.Health == 0 || b.s.Round >= MaxPetRounds
}

// Won tells whether the attacker won: the defender fell, or, when the
// rounds are over, the attacker kept the greater share of its health —
// the defender wins a tie.
func (b *PetBattle) Won() bool {
	a, d := b.s.Attacker, b.s.Defender
	switch {
	case d.Health == 0:
		return true
	case a.Health == 0:
		return false
	}

	return a.Health*d.Fighter.Health > d.Health*a.Fighter.Health
}

// Bout is the fight as the list of fights keeps it, the rewards of a win
// at win votes.
func (b *PetBattle) Bout(win int64) PetBout {
	s := b.State()
	bout := PetBout{Attacker: s.Attacker.Fighter, Defender: s.Defender.Fighter, Hits: s.Hits, Won: b.Won(), Rating: -s.Loss, Ratio: s.Ratio}
	if bout.Won {
		bout.Rating = s.Win
		bout.Reward = win * int64(s.Ratio) / 100
	}

	return bout
}

// combatant is p in a fight at now in stance with stats.
func (p *Pet) combatant(stance PetStance, stats PetStats, fatigue int) PetCombatant {
	return PetCombatant{
		Fighter: PetFighter{
			Kind: p.s.Kind, Name: p.s.Name, Level: p.Level(), Health: stats.Health, Outfit: p.Outfit(),
			Stance: stance, Power: stats.Power(), Fatigue: fatigue,
		},
		Stats:  stats,
		Health: stats.Health,
	}
}

// Challenged starts a fight of the pet in stance — the one it defends in
// when empty — against defender at now:
// the pet and the defender after the start, and the fight. The fight is
// counted as lost from its start, and Settled turns it into a win: a fight
// left halfway is lost. The rewards follow the power of the defender to
// the pet's: little for beating the weak, more for beating the strong.
func (p *Pet) Challenged(defender *Pet, stance PetStance, shop PetShop, now time.Time) (*Pet, *Pet, *PetBattle, error) {
	if err := p.CheckFightAt(shop, now); err != nil {
		return nil, nil, nil, err
	}
	if stance == "" {
		stance = p.s.Stance
	}

	fatigue := p.FatigueAt(now)
	as, ds := p.FightStatsAt(now), defender.Stats()
	ratio := min(max(ds.Power()*100/max(as.Power(), 1), petRatioMin), petRatioMax)
	win := eloChange(p.s.Rating, defender.s.Rating, true) * ratio / 100
	lost := -eloChange(p.s.Rating, defender.s.Rating, false)
	loss := min(max(lost*100/ratio, 1), 2*lost)

	a := p.counted(now)
	a.Satiety = max(a.Satiety-PetFightTakes, 0)
	a.Joy = max(a.Joy-PetFightTakes, 0)
	a.Fights = p.FightsAt(now) + 1
	a.FoughtOn = BonusDay(now)
	a.Streak, a.StreakAt = min(fatigue+1, MaxPetFatigue), now
	a.Rating -= loss
	a.Losses++
	a.Experience += PetLossGain

	d := defender.State()
	d.Rating += loss
	d.Wins++
	d.Experience += PetDefendGain

	battle := &PetBattle{s: PetBattleState{
		PetID: p.s.ID, DefenderPet: defender.s.ID,
		Attacker: p.combatant(stance, as, fatigue), Defender: defender.combatant(defender.s.Stance, ds, 0),
		Ratio: ratio, Win: win, Loss: loss, WinGain: max(int64(PetWinGain*ratio/100), 1), StartedAt: now,
	}}

	return &Pet{s: a}, &Pet{s: d}, battle, nil
}

// Against is the battle against the pet of owner.
func (b *PetBattle) Against(owner uuid.UUID) *PetBattle {
	s := b.State()
	s.DefenderOwner = owner

	return &PetBattle{s: s}
}

// Settled are the attacker and the defender after the battle is over:
// a win takes back the loss counted at the start and counts the win;
// defender is nil when it is gone. ErrPetInBattle before the end.
func (b *PetBattle) Settled(attacker, defender *Pet) (*Pet, *Pet, error) {
	if !b.Done() {
		return nil, nil, ErrPetInBattle
	}
	if !b.Won() {
		return attacker, defender, nil
	}

	a := attacker.State()
	a.Rating += b.s.Loss + b.s.Win
	a.Losses = max(a.Losses-1, 0)
	a.Wins++
	a.Experience += b.s.WinGain - PetLossGain
	won := &Pet{s: a}

	if defender == nil {
		return won, nil, nil
	}

	d := defender.State()
	d.Rating -= b.s.Loss + b.s.Win
	d.Wins = max(d.Wins-1, 0)
	d.Losses++

	return won, &Pet{s: d}, nil
}

// Moved is the battle after the attacker makes move: a round of it, or,
// with PetAuto, all the rounds left. ErrPetBattleOver after the end.
func (b *PetBattle) Moved(move PetMove, dice PetDice) (*PetBattle, error) {
	if b.Done() {
		return nil, ErrPetBattleOver
	}

	n := &PetBattle{s: b.State()}
	for !n.Done() {
		m := move
		if move == PetAuto {
			m = n.s.Attacker.plan(n.s.Round)
		}
		n.round(m, &dice)

		if move != PetAuto {
			break
		}
	}

	return n, nil
}

// Fought is the whole fight of the pet in stance against defender at now,
// by their stances: the pet and the defender after it and how it went.
func (p *Pet) Fought(defender *Pet, stance PetStance, shop PetShop, now time.Time, dice PetDice) (*Pet, *Pet, PetBout, error) {
	a, d, battle, err := p.Challenged(defender, stance, shop, now)
	if err != nil {
		return nil, nil, PetBout{}, err
	}

	if battle, err = battle.Moved(PetAuto, dice); err != nil {
		return nil, nil, PetBout{}, err
	}

	a, d, err = battle.Settled(a, d)

	return a, d, battle.Bout(shop.Win()), err
}

// plan is the move of a pet led by its stance in round: one in attack
// strikes hard each third round, one in defense guards.
func (c *PetCombatant) plan(round int) PetMove {
	switch {
	case c.Fighter.Stance == PetAttack && round%3 == 2:
		return PetPower
	case c.Fighter.Stance == PetDefense && round%3 == 1:
		return PetGuard
	}

	return PetStrike
}

func (c *PetCombatant) used(s PetSpecial) bool {
	return slices.Contains(c.Used, s)
}

func (c *PetCombatant) use(s PetSpecial) {
	c.Used = append(c.Used, s)
}

// low tells whether less than 30% of its health is left.
func (c *PetCombatant) low() bool {
	return c.Health*10 < c.Fighter.Health*3
}

func (c *PetCombatant) heal(n int) int {
	n = min(n, c.Fighter.Health-c.Health)
	c.Health += n

	return n
}

// sides are the one who acts and the other.
func (b *PetBattle) sides(byAttacker bool) (actor, other *PetCombatant) {
	if byAttacker {
		return &b.s.Attacker, &b.s.Defender
	}

	return &b.s.Defender, &b.s.Attacker
}

// round is a round of the battle: the faster pet acts first, the attacker
// by move, the defender by its stance.
func (b *PetBattle) round(move PetMove, dice *PetDice) {
	first := b.s.Attacker.Stats.Speed >= b.s.Defender.Stats.Speed
	for _, byAttacker := range []bool{first, !first} {
		if b.s.Attacker.Health == 0 || b.s.Defender.Health == 0 {
			break
		}

		m := b.s.Defender.plan(b.s.Round)
		if byAttacker {
			m = move
		}
		b.act(byAttacker, m, dice)
	}
	b.s.Round++
}

// act is what one pet does in a round.
func (b *PetBattle) act(byAttacker bool, move PetMove, dice *PetDice) {
	actor, other := b.sides(byAttacker)
	// A guard lasts until the pet acts again.
	actor.Guarding = false

	if actor.Fighter.Kind.Special() == PetCheeks && actor.low() && !actor.used(PetCheeks) {
		actor.use(PetCheeks)
		b.record(PetHit{ByAttacker: byAttacker, Move: move, Special: PetCheeks, Heal: actor.heal(actor.Fighter.Health / 4)})

		return
	}

	if move == PetGuard {
		actor.Guarding, actor.Charged = true, true
		b.record(PetHit{ByAttacker: byAttacker, Move: PetGuard, Heal: actor.heal(max(actor.Fighter.Health*petGuardHeals/100, 1))})

		return
	}

	b.strike(byAttacker, move, "", dice)

	if actor.Fighter.Kind.Special() == PetLeap && actor.Strikes == 1 && other.Health > 0 && !actor.used(PetLeap) {
		actor.use(PetLeap)
		b.strike(byAttacker, PetStrike, PetLeap, dice)
	}
}

// dodge is the chance in a hundred of other to dodge a hit of move.
func (b *PetBattle) dodge(other *PetCombatant, move PetMove) (chance int, sight bool) {
	chance = other.Stats.Speed
	if other.Fighter.Stance == PetAgility {
		chance += petAgilityDodges
	}
	if other.Fighter.Kind.Special() == PetSight && b.s.Round*2 >= MaxPetRounds {
		chance += petSightDodges
		sight = true
	}
	if move == PetPower {
		chance += petPowerDodged
	}

	return min(chance, MaxPetDodge), sight
}

// strike is a hit of move by one pet at the other.
func (b *PetBattle) strike(byAttacker bool, move PetMove, special PetSpecial, dice *PetDice) {
	actor, other := b.sides(byAttacker)
	actor.Strikes++
	hit := PetHit{ByAttacker: byAttacker, Move: move, Special: special}

	if other.Fighter.Kind.Special() == PetTrick && !other.used(PetTrick) {
		other.use(PetTrick)
		hit.Dodged, hit.Answer = true, PetTrick
		b.record(hit)

		return
	}

	// High numbers dodge and crit, so dice of zeros never do.
	chance, sight := b.dodge(other, move)
	if dice.next(100) >= 100-chance {
		hit.Dodged = true
		if sight {
			hit.Answer = PetSight
		}
		b.record(hit)

		if other.Fighter.Kind.Special() == PetCounter {
			b.counter(!byAttacker, dice)
		}

		return
	}

	defense := other.Stats.Defense
	if actor.Fighter.Kind.Special() == PetFlame && actor.Strikes%3 == 0 {
		defense, hit.Special = 0, PetFlame
	}

	damage := max(actor.Stats.Attack*2-defense+dice.next(actor.Stats.Attack+1), 1)
	damage = damage * b.boost(actor, other, move, &hit) / 100
	if dice.next(100) >= 100-critOf(actor.Fighter.Kind) {
		damage, hit.Crit = damage*2, true
	}

	hit.Damage = b.taken(other, damage, &hit)
	b.wound(other, hit.Damage, &hit)

	if hit.Crit && actor.Fighter.Kind.Special() == PetRainbow {
		hit.Special, hit.Heal = PetRainbow, actor.heal(hit.Damage/2)
	}

	b.record(hit)
}

// boost is the percent a hit of actor at other with move deals.
func (b *PetBattle) boost(actor, other *PetCombatant, move PetMove, hit *PetHit) int {
	pct := 100
	times := func(p int) { pct = pct * p / 100 }

	if actor.Fighter.Stance == PetAttack {
		times(petAttackHits)
	}
	if other.Fighter.Stance == PetDefense {
		times(petDefenseTakes)
	}
	if actor.Fighter.Stance.Beats(other.Fighter.Stance) {
		times(petBeats)
	}
	if actor.Fighter.Kind.Element().Beats(other.Fighter.Kind.Element()) {
		times(petBeats)
	}
	if move == PetPower {
		times(petPowerHits)
	}
	if actor.Charged {
		times(petGuardCharge)
		actor.Charged = false
	}

	switch actor.Fighter.Kind.Special() {
	case PetRage:
		if actor.low() {
			times(petRageHits)
			hit.Special = PetRage
		}
	case PetHowl:
		if steps := min(actor.Strikes-1, petHowlSteps); steps > 0 {
			times(100 + petHowlStep*steps)
			hit.Special = PetHowl
		}
	}

	return pct
}

// taken is what other takes of damage: less on guard and for the panda,
// nothing for the turtle that hides.
func (b *PetBattle) taken(other *PetCombatant, damage int, hit *PetHit) int {
	if other.Guarding {
		damage = damage * petGuardTakes / 100
		other.Guarding = false
	}
	if other.Fighter.Kind.Special() == PetHide {
		damage -= petHideTakes
	}
	damage = max(damage, 1)

	if other.Fighter.Kind.Special() == PetShell && !other.used(PetShell) && other.Health*2 < other.Fighter.Health {
		other.use(PetShell)
		hit.Answer = PetShell

		return 0
	}

	return damage
}

// wound takes damage of the health of other: the phoenix that falls rises
// once.
func (b *PetBattle) wound(other *PetCombatant, damage int, hit *PetHit) {
	other.Health = max(other.Health-damage, 0)
	if other.Health == 0 && other.Fighter.Kind.Special() == PetRebirth && !other.used(PetRebirth) {
		other.use(PetRebirth)
		other.Health = max(other.Fighter.Health/4, 1)
		hit.Answer = PetRebirth
	}
}

// counter is the cat striking back at half strength after it dodged.
func (b *PetBattle) counter(byCat bool, dice *PetDice) {
	cat, other := b.sides(byCat)
	if cat.Health == 0 || other.Health == 0 {
		return
	}

	hit := PetHit{ByAttacker: byCat, Move: PetStrike, Special: PetCounter}
	hit.Damage = max((cat.Stats.Attack*2-other.Stats.Defense+dice.next(cat.Stats.Attack+1))/2, 1)
	b.wound(other, hit.Damage, &hit)
	b.record(hit)
}

// record keeps hit with the health both pets have after it.
func (b *PetBattle) record(hit PetHit) {
	hit.AttackerHealth, hit.DefenderHealth = b.s.Attacker.Health, b.s.Defender.Health
	b.s.Hits = append(b.s.Hits, hit)
}

// eloChange is the points a pet of rating a wins or loses against one of
// rating b: at least one either way.
func eloChange(a, b int, won bool) int {
	expected := 1 / (1 + math.Pow(10, float64(b-a)/400))

	score := 0.0
	if won {
		score = 1
	}

	change := int(math.Round(PetEloFactor * (score - expected)))
	switch {
	case won && change < 1:
		return 1
	case !won && change > -1:
		return -1
	}

	return change
}

// PetContender is a pet that may be set on another, and its owner.
type PetContender struct {
	Owner uuid.UUID
	Pet   *Pet
}

// How opponents are matched: by power within petMatchClose percent, then
// within petMatchFar, then the nearest at all; one of petMatchPick.
const (
	petMatchClose = 15
	petMatchFar   = 30
	petMatchPick  = 5
)

// PickPetOpponent is the opponent of own among contenders: one of the
// nearest in power, close to it as can be; pick is any number from 0 that
// chooses among them. ErrNoPetOpponent without contenders.
func PickPetOpponent(own *Pet, contenders []PetContender, pick int64) (PetContender, error) {
	if len(contenders) == 0 {
		return PetContender{}, ErrNoPetOpponent
	}

	power := own.Power()
	gap := func(c PetContender) int {
		d := c.Pet.Power() - power

		return max(d, -d)
	}

	sorted := slices.Clone(contenders)
	slices.SortStableFunc(sorted, func(a, b PetContender) int { return gap(a) - gap(b) })

	within := func(percent int) []PetContender {
		n := 0
		for n < len(sorted) && gap(sorted[n])*100 <= power*percent {
			n++
		}

		return sorted[:n]
	}

	near := within(petMatchClose)
	if len(near) == 0 {
		near = within(petMatchFar)
	}
	if len(near) == 0 {
		near = sorted[:1]
	}
	near = near[:min(len(near), petMatchPick)]

	return near[pick%int64(len(near))], nil
}
