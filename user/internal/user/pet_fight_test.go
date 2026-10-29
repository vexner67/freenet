package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

var fightNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

// fighter is a fed and glad pet of kind at level.
func fighter(kind user.PetKind, level int) *user.Pet {
	return user.RestorePet(user.PetState{
		ID: uuid.Must(uuid.NewV7()), Kind: kind, Name: "Боец", Satiety: 100, Joy: 100, CountedAt: fightNow,
		Experience: user.PetLevelExperience(level), Rating: user.PetStartRating, Stance: user.PetDefense,
	})
}

// dice are n dice of v.
func dice(v int64) user.PetDice {
	d := make(user.PetDice, user.PetDiceCount)
	for i := range d {
		d[i] = v
	}

	return d
}

func TestPet_Fought(t *testing.T) {
	t.Parallel()

	shop := user.DefaultPetShop()
	dragon, hamster := fighter(user.PetDragon, 10), fighter(user.PetHamster, 1)

	// With zero dice nobody dodges and the stronger dragon wins, but a
	// win over the weak brings little and losing to it costs more.
	a, d, bout, err := dragon.Fought(hamster, user.PetAttack, shop, fightNow, nil)
	if err != nil || !bout.Won || bout.Ratio != 33 || bout.Rating != 5 || bout.Reward != 0 {
		t.Fatalf("Fought = %+v, %v", bout, err)
	}
	if last := bout.Hits[len(bout.Hits)-1]; last.DefenderHealth != 0 || !last.ByAttacker {
		t.Fatalf("the last hit: %+v", last)
	}
	if a.Rating() != 1005 || d.Rating() != 995 || a.Wins() != 1 || a.Losses() != 0 || d.Losses() != 1 || d.Wins() != 0 {
		t.Fatalf("after the fight: %d %d %d %d", a.Rating(), d.Rating(), a.Wins(), d.Losses())
	}
	if a.Experience() != dragon.Experience()+6 || d.Experience() != user.PetDefendGain {
		t.Fatalf("the experience: %d %d", a.Experience(), d.Experience())
	}
	if a.SatietyAt(fightNow) != 90 || a.FightsAt(fightNow) != 1 || a.FightsAt(fightNow.Add(24*time.Hour)) != 0 {
		t.Fatalf("what the fight took: %d %d", a.SatietyAt(fightNow), a.FightsAt(fightNow))
	}
	if bout.Attacker.Stance != user.PetAttack || bout.Defender.Stance != user.PetDefense || bout.Attacker.Power != dragon.Power() {
		t.Fatalf("the fighters: %+v %+v", bout.Attacker, bout.Defender)
	}

	// The weak hamster loses little to the dragon, and would win much.
	if _, _, bout, err = hamster.Fought(dragon, user.PetAttack, shop, fightNow, nil); err != nil || bout.Won || bout.Rating != -8 || bout.Ratio != 200 {
		t.Fatalf("the hamster fought: %+v, %v", bout, err)
	}

	hungry := user.RestorePet(user.PetState{Kind: user.PetCat, Satiety: 10, Joy: 100, CountedAt: fightNow})
	if _, _, _, err = hungry.Fought(hamster, user.PetAttack, shop, fightNow, nil); !errors.Is(err, user.ErrPetHungry) {
		t.Fatalf("a hungry pet fought: %v", err)
	}

	// Dice of the top numbers dodge every hit: the rounds end and the
	// defender keeps it on a tie.
	_, _, bout, _ = dragon.Fought(hamster, user.PetAttack, shop, fightNow, dice(99))
	if bout.Won || bout.Hits[len(bout.Hits)-1].DefenderHealth != bout.Defender.Health {
		t.Fatalf("a fight of dodges: %+v", bout)
	}

	once, err := user.NewPetShop(shop.Prices(), 5, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = a.Fought(hamster, user.PetAttack, once, fightNow, nil); !errors.Is(err, user.ErrPetFightsOver) {
		t.Fatalf("the fights of a day: %v", err)
	}
}

func TestPet_Fatigue(t *testing.T) {
	t.Parallel()

	wolf := fighter(user.PetWolf, 5)
	tired, _, _, err := wolf.Fought(fighter(user.PetCat, 1), user.PetAttack, user.DefaultPetShop(), fightNow, nil)
	if err != nil {
		t.Fatal(err)
	}

	if tired.FatigueAt(fightNow) != 1 || tired.FightStatsAt(fightNow).Attack != wolf.Stats().Attack*95/100 {
		t.Fatalf("after a fight: %d, %+v", tired.FatigueAt(fightNow), tired.FightStatsAt(fightNow))
	}
	if rested := fightNow.Add(user.PetRest); tired.FatigueAt(rested) != 0 || tired.FightStatsAt(rested) != wolf.Stats() {
		t.Fatalf("after a rest: %d", tired.FatigueAt(rested))
	}
}

func TestPet_LevelStats(t *testing.T) {
	t.Parallel()

	// Levels give the common kinds more than the legendary ones.
	cat, phoenix := fighter(user.PetCat, 20), fighter(user.PetPhoenix, 20)
	if cat.LevelStats().Attack != 38 || phoenix.LevelStats().Attack != 26 {
		t.Fatalf("level stats: %+v, %+v", cat.LevelStats(), phoenix.LevelStats())
	}
	// A cat of level 20 is stronger than a phoenix of level 5.
	if cat.Power() <= fighter(user.PetPhoenix, 5).Power() {
		t.Fatalf("powers: %d, %d", cat.Power(), fighter(user.PetPhoenix, 5).Power())
	}
}

func TestPetStance(t *testing.T) {
	t.Parallel()

	for _, s := range user.PetStances {
		beats := 0
		for _, o := range user.PetStances {
			if s.Beats(o) {
				beats++
			}
		}
		if beats != 1 {
			t.Errorf("%s beats %d stances", s, beats)
		}
	}
	if !user.PetAttack.Beats(user.PetAgility) || !user.PetWater.Beats(user.PetFire) || user.PetFire.Beats(user.PetWater) {
		t.Error("the circles are wrong")
	}
	if _, err := user.NewPetStance("rage"); !errors.Is(err, user.ErrUnknownPetStance) {
		t.Errorf("NewPetStance: %v", err)
	}
	if _, err := user.NewPetMove("run"); !errors.Is(err, user.ErrUnknownPetMove) {
		t.Errorf("NewPetMove: %v", err)
	}

	elements := map[user.PetElement]int{}
	for _, k := range user.PetKinds {
		elements[k.Element()]++
		if k.Special() == "" {
			t.Errorf("%s has no special", k)
		}
	}
	if len(elements) != len(user.PetElements) {
		t.Errorf("elements: %v", elements)
	}
}

// specialIn tells whether a hit of bout has special of its actor or as
// the answer of the other.
func specialIn(bout user.PetBout, special user.PetSpecial) bool {
	for _, h := range bout.Hits {
		if h.Special == special || h.Answer == special {
			return true
		}
	}

	return false
}

func TestPetSpecials(t *testing.T) {
	t.Parallel()

	shop := user.DefaultPetShop()
	tests := []struct {
		name     string
		attacker *user.Pet
		defender *user.Pet
		dice     user.PetDice
		special  user.PetSpecial
	}{
		{"the phoenix rises", fighter(user.PetWolf, 30), fighter(user.PetPhoenix, 1), nil, user.PetRebirth},
		{"the turtle hides", fighter(user.PetWolf, 1), fighter(user.PetTurtle, 1), nil, user.PetShell},
		{"the fox tricks", fighter(user.PetWolf, 1), fighter(user.PetFox, 1), nil, user.PetTrick},
		{"the rabbit leaps", fighter(user.PetRabbit, 1), fighter(user.PetDog, 1), nil, user.PetLeap},
		{"the cat strikes back", fighter(user.PetDog, 1), fighter(user.PetCat, 1), dice(99), user.PetCounter},
		{"the dragon burns", fighter(user.PetDragon, 1), fighter(user.PetPanda, 10), nil, user.PetFlame},
		{"the wolf howls", fighter(user.PetWolf, 1), fighter(user.PetPanda, 10), nil, user.PetHowl},
		{"the hamster eats", fighter(user.PetHamster, 1), fighter(user.PetWolf, 5), nil, user.PetCheeks},
		{"the dog rages", fighter(user.PetDog, 1), fighter(user.PetWolf, 4), nil, user.PetRage},
		{"the owl sees", fighter(user.PetDog, 1), fighter(user.PetOwl, 1), dice(99), user.PetSight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, bout, err := tt.attacker.Fought(tt.defender, user.PetAttack, shop, fightNow, tt.dice)
			if err != nil || !specialIn(bout, tt.special) {
				t.Fatalf("Fought = %+v, %v", bout, err)
			}
		})
	}

	// The leap is a second hit of the first act of the rabbit.
	_, _, bout, _ := fighter(user.PetRabbit, 1).Fought(fighter(user.PetDog, 1), user.PetAttack, shop, fightNow, nil)
	if !bout.Hits[0].ByAttacker || bout.Hits[1].Special != user.PetLeap || !bout.Hits[1].ByAttacker {
		t.Fatalf("the leap: %+v", bout.Hits[:2])
	}
}

func TestPetBattle(t *testing.T) {
	t.Parallel()

	shop := user.DefaultPetShop()
	wolf, dog := fighter(user.PetWolf, 3), fighter(user.PetDog, 3)

	// The start counts a loss: a fight left halfway is lost.
	a, d, battle, err := wolf.Challenged(dog, user.PetAgility, shop, fightNow)
	if err != nil || battle.Done() {
		t.Fatalf("Challenged = %v", err)
	}
	win, loss := battle.Rating()
	if a.Rating() != 1000-loss || d.Rating() != 1000+loss || a.Losses() != 1 || d.Wins() != 1 || a.FightsAt(fightNow) != 1 {
		t.Fatalf("at the start: %d %d", a.Rating(), d.Rating())
	}
	if _, _, err = battle.Settled(a, d); !errors.Is(err, user.ErrPetInBattle) {
		t.Fatalf("Settled before the end: %v", err)
	}

	// A guard heals and charges; the next strike is harder.
	guarded, err := battle.Moved(user.PetGuard, nil)
	if err != nil || guarded.Round() != 1 || len(battle.Hits()) != 0 {
		t.Fatalf("Moved = %v, %d", err, guarded.Round())
	}
	if hits := guarded.Hits(); hits[0].Move != user.PetGuard && hits[1].Move != user.PetGuard {
		t.Fatalf("the guard: %+v", hits)
	}

	done, err := guarded.Moved(user.PetAuto, nil)
	if err != nil || !done.Done() {
		t.Fatalf("auto: %v", err)
	}
	if _, err = done.Moved(user.PetStrike, nil); !errors.Is(err, user.ErrPetBattleOver) {
		t.Fatalf("a move after the end: %v", err)
	}

	sa, sd, err := done.Settled(a, d)
	if err != nil {
		t.Fatal(err)
	}
	bout := done.Bout(10)
	if bout.Won {
		if sa.Rating() != 1000+win || sd.Rating() != 1000-win || sa.Wins() != 1 || sa.Losses() != 0 || bout.Reward != 10*int64(bout.Ratio)/100 {
			t.Fatalf("a won battle: %d %d %+v", sa.Rating(), sd.Rating(), bout)
		}
	} else if sa.Rating() != 1000-loss || bout.Reward != 0 {
		t.Fatalf("a lost battle: %d %+v", sa.Rating(), bout)
	}

	// The defender may be gone by the end.
	if _, gone, err := done.Settled(a, nil); err != nil || gone != nil {
		t.Fatalf("Settled without the defender: %v", err)
	}
}

func TestPickPetOpponent(t *testing.T) {
	t.Parallel()

	own := fighter(user.PetCat, 10)
	if _, err := user.PickPetOpponent(own, nil, 0); !errors.Is(err, user.ErrNoPetOpponent) {
		t.Fatalf("no contenders: %v", err)
	}

	near := user.PetContender{Owner: uuid.New(), Pet: fighter(user.PetCat, 10)}
	far := user.PetContender{Owner: uuid.New(), Pet: fighter(user.PetPhoenix, 1)}
	strong := user.PetContender{Owner: uuid.New(), Pet: fighter(user.PetPhoenix, 30)}

	for pick := range int64(3) {
		got, err := user.PickPetOpponent(own, []user.PetContender{strong, far, near}, pick)
		if err != nil || got.Owner != near.Owner {
			t.Fatalf("pick %d: %v, %v", pick, got.Owner, err)
		}
	}

	// Without a near one the nearest of all fights.
	if got, _ := user.PickPetOpponent(own, []user.PetContender{strong}, 7); got.Owner != strong.Owner {
		t.Fatalf("the nearest: %v", got.Owner)
	}
}

func TestPetKeeper(t *testing.T) {
	t.Parallel()

	k := user.NewPetKeeper()
	if _, err := k.Opened(user.PetCaseGold, true, 0, 0, fightNow); !errors.Is(err, user.ErrNotEnoughShards) {
		t.Fatalf("a case without shards: %v", err)
	}

	opened, err := k.Opened(user.PetCaseBronze, false, 0, 0, fightNow)
	if err != nil || opened.Drop() != user.PetCat || opened.DropCase() != user.PetCaseBronze || opened.Pity() != 1 || opened.CasesAt(fightNow) != 1 {
		t.Fatalf("Opened = %+v, %v", opened.State(), err)
	}
	if _, err = opened.Opened(user.PetCaseBronze, false, 0, 0, fightNow); !errors.Is(err, user.ErrPetDropWaiting) {
		t.Fatalf("a case with a pet waiting: %v", err)
	}

	left, err := opened.Left()
	if err != nil || left.Shards() != 1 || left.Drop() != "" {
		t.Fatalf("Left = %+v, %v", left.State(), err)
	}
	if _, err = left.Left(); !errors.Is(err, user.ErrNoPetDrop) {
		t.Fatalf("Left without a pet: %v", err)
	}

	// The PetPity-th case without a legendary pet gives one.
	unlucky := user.RestorePetKeeper(user.PetKeeperState{Pity: user.PetPity - 1, Shards: 60})
	lucky, err := unlucky.Opened(user.PetCaseGold, true, 0, 1, fightNow)
	if err != nil || lucky.Drop() != user.PetPhoenix || lucky.Pity() != 0 || lucky.Shards() != 0 {
		t.Fatalf("the pity: %+v, %v", lucky.State(), err)
	}
	took, kind, err := lucky.Took()
	if err != nil || kind != user.PetPhoenix || took.Drop() != "" || took.Released(kind).Shards() != 20 {
		t.Fatalf("Took = %v, %v", kind, err)
	}

	full := user.RestorePetKeeper(user.PetKeeperState{CasesOn: user.BonusDay(fightNow), Cases: user.MaxPetCasesADay})
	if _, err = full.Opened(user.PetCaseBronze, false, 0, 0, fightNow); !errors.Is(err, user.ErrPetCasesOver) {
		t.Fatalf("the cases of a day: %v", err)
	}
	if _, err = full.Opened(user.PetCaseBronze, false, 0, 0, fightNow.Add(24*time.Hour)); err != nil {
		t.Fatalf("the cases of the next day: %v", err)
	}
}
