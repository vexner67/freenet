package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewMine(t *testing.T) {
	t.Parallel()

	def := user.DefaultMine()
	if _, err := user.NewMine(def.Veins(), def.Energy(), def.Regen(), def.Events()); err != nil {
		t.Fatalf("NewMine of the default mine: %v", err)
	}

	tests := []struct {
		name   string
		change func(veins []user.OreVein, energy *int, regen *time.Duration, events *user.MineEvents) []user.OreVein
		want   error
	}{
		{"an ore missing", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein { return v[1:] }, user.ErrMineVeins},
		{"ores out of order", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[0], v[1] = v[1], v[0]
			return v
		}, user.ErrMineVeins},
		{"stone needs experience", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[0].Experience = 1
			return v
		}, user.ErrOreExperience},
		{"experience goes down", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[2].Experience = v[1].Experience - 1
			return v
		}, user.ErrOreExperience},
		{"no yield", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[1].Least = 0
			return v
		}, user.ErrOreYield},
		{"least above most", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[1].Least = v[1].Most + 1
			return v
		}, user.ErrOreYield},
		{"negative price", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[1].Price = -1
			return v
		}, user.ErrOrePrice},
		{"negative gain", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[1].Gain = -1
			return v
		}, user.ErrOreGain},
		{"a dig for nothing", func(v []user.OreVein, _ *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			v[1].Energy = 0
			return v
		}, user.ErrOreEnergy},
		{"a dig above the energy", func(v []user.OreVein, e *int, _ *time.Duration, _ *user.MineEvents) []user.OreVein {
			*e = 2
			return v
		}, user.ErrMineEnergy},
		{"too fast", func(v []user.OreVein, _ *int, r *time.Duration, _ *user.MineEvents) []user.OreVein {
			*r = time.Millisecond
			return v
		}, user.ErrMineRegen},
		{"chances above all", func(v []user.OreVein, _ *int, _ *time.Duration, e *user.MineEvents) []user.OreVein {
			e.Collapse = user.MineOdds
			return v
		}, user.ErrMineChances},
		{"a nugget of one", func(v []user.OreVein, _ *int, _ *time.Duration, e *user.MineEvents) []user.OreVein {
			e.NuggetTimes = 1
			return v
		}, user.ErrMineNugget},
		{"a chest too big", func(v []user.OreVein, _ *int, _ *time.Duration, e *user.MineEvents) []user.OreVein {
			e.ChestMost = user.MaxChestVotes + 1
			return v
		}, user.ErrMineChest},
		{"a drink of nothing", func(v []user.OreVein, _ *int, _ *time.Duration, e *user.MineEvents) []user.OreVein {
			e.DrinkEnergy = 0
			return v
		}, user.ErrMineDrinkEnergy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			energy, regen, events := def.Energy(), def.Regen(), def.Events()
			veins := tt.change(def.Veins(), &energy, &regen, &events)

			if _, err := user.NewMine(veins, energy, regen, events); !errors.Is(err, tt.want) {
				t.Fatalf("NewMine = %v; want %v", err, tt.want)
			}
		})
	}
}

func TestNewOre(t *testing.T) {
	t.Parallel()

	if o, err := user.NewOre("gold"); err != nil || o != user.OreGold {
		t.Fatalf("NewOre(gold) = %v, %v", o, err)
	}
	if _, err := user.NewOre("tin"); !errors.Is(err, user.ErrUnknownOre) {
		t.Fatalf("NewOre(tin) = %v", err)
	}
}

func TestMiner_Energy(t *testing.T) {
	t.Parallel()

	mine := user.DefaultMine()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	m := user.NewMiner(mine, now)

	if m.Energy() != mine.Energy() || m.NextEnergy(mine, now) != 0 {
		t.Fatalf("a new miner has %d energy, next in %v", m.Energy(), m.NextEnergy(mine, now))
	}

	m, _, err := m.Dig(mine, user.OreStone, now, user.MineRolls{Event: user.MineOdds - 1})
	if err != nil {
		t.Fatal(err)
	}
	if m.Energy() != mine.Energy()-1 || !m.CountedAt().Equal(now) {
		t.Fatalf("after a dig: %d energy counted at %v", m.Energy(), m.CountedAt())
	}

	// A point comes back after the regen, the part of the next one stays.
	later := now.Add(mine.Regen() + time.Minute)
	if got := m.At(mine, later); got.Energy() != mine.Energy() || !got.CountedAt().Equal(later) {
		t.Fatalf("At after a regen = %d at %v", got.Energy(), got.CountedAt())
	}
	if next := m.NextEnergy(mine, now.Add(time.Minute)); next != mine.Regen()-time.Minute {
		t.Fatalf("NextEnergy = %v", next)
	}

	// Digging the energy out.
	for m.At(mine, now).Energy() > 0 {
		if m, _, err = m.Dig(mine, user.OreStone, now, user.MineRolls{Event: user.MineOdds - 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = m.Dig(mine, user.OreStone, now, user.MineRolls{}); !errors.Is(err, user.ErrNoEnergy) {
		t.Fatalf("Dig without energy: %v", err)
	}

	two := now.Add(2*mine.Regen() + time.Second)
	if got := m.At(mine, two); got.Energy() != 2 || !got.CountedAt().Equal(now.Add(2*mine.Regen())) {
		t.Fatalf("At after two regens = %d at %v", got.Energy(), got.CountedAt())
	}
}

func TestMiner_Dig(t *testing.T) {
	t.Parallel()

	mine := user.DefaultMine()
	stone, _ := mine.Vein(user.OreStone)
	events := mine.Events()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	nothing := int64(user.MineOdds - 1)

	m := user.NewMiner(mine, now)
	if _, _, err := m.Dig(mine, user.OreCoal, now, user.MineRolls{}); !errors.Is(err, user.ErrOreLocked) {
		t.Fatalf("Dig of coal without experience: %v", err)
	}
	if _, _, err := m.Dig(mine, "tin", now, user.MineRolls{}); !errors.Is(err, user.ErrUnknownOre) {
		t.Fatalf("Dig of tin: %v", err)
	}

	tests := []struct {
		name   string
		roll   int64
		units  int64
		votes  int64
		gain   int64
		energy int
	}{
		{"plain", nothing, stone.Least + 5, 0, stone.Gain, mine.Energy() - 1},
		{"nugget", 0, (stone.Least + 5) * int64(events.NuggetTimes), 0, stone.Gain, mine.Energy() - 1},
		{"chest", int64(events.Nugget), stone.Least + 5, events.ChestLeast + 7, stone.Gain, mine.Energy() - 1},
		{"collapse", int64(events.Nugget + events.Chest), 0, 0, 0, mine.Energy() - 1},
		{"drink", int64(events.Nugget + events.Chest + events.Collapse), stone.Least + 5, 0, stone.Gain, mine.Energy()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, haul, err := m.Dig(mine, user.OreStone, now, user.MineRolls{Event: tt.roll, Yield: 5, Chest: 7})
			if err != nil {
				t.Fatal(err)
			}
			if haul.Units != tt.units || haul.Votes != tt.votes || haul.Experience != tt.gain {
				t.Fatalf("haul = %+v", haul)
			}
			if got.Backpack()[user.OreStone] != tt.units || got.Experience() != tt.gain || got.Earned() != tt.votes ||
				got.Digs() != 1 || got.Energy() != tt.energy {
				t.Fatalf("miner = %+v", got)
			}
		})
	}

	// The miner itself is not changed.
	if len(m.Backpack()) != 0 || m.Digs() != 0 {
		t.Fatalf("Dig changed the miner: %+v", m)
	}
}

func TestMiner_Sell(t *testing.T) {
	t.Parallel()

	mine := user.DefaultMine()
	stone, _ := mine.Vein(user.OreStone)
	m := user.RestoreMiner(5, time.Now(), 0, map[user.Ore]int64{user.OreStone: 149, user.OreCoal: 0}, 1, 0)

	got, units, votes, err := m.Sell(mine, user.OreStone)
	if err != nil || units != 149 || votes != 149*stone.Price/100 {
		t.Fatalf("Sell = %d, %d, %v", units, votes, err)
	}
	if _, ok := got.Backpack()[user.OreStone]; ok || got.Earned() != votes {
		t.Fatalf("after Sell: %+v", got)
	}
	if m.Backpack()[user.OreStone] != 149 {
		t.Fatal("Sell changed the miner")
	}

	if _, _, _, err = m.Sell(mine, user.OreCoal); !errors.Is(err, user.ErrNothingToSell) {
		t.Fatalf("Sell of nothing: %v", err)
	}
	little := user.RestoreMiner(5, time.Now(), 0, map[user.Ore]int64{user.OreStone: 49}, 1, 0)
	if _, _, _, err = little.Sell(mine, user.OreStone); !errors.Is(err, user.ErrNothingToSell) {
		t.Fatalf("Sell of less than a vote: %v", err)
	}
}
