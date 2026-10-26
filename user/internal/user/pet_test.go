package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewPetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{name: "plain", in: " Барсик ", want: "Барсик"},
		{name: "emoji", in: "Кекс 🐱", want: "Кекс 🐱"},
		{name: "empty", in: "  ", err: user.ErrPetNameEmpty},
		{name: "lines", in: "Бар\nсик", err: user.ErrPetNameLines},
		{name: "long", in: strings.Repeat("я", 21), err: user.ErrPetNameTooLong},
		{name: "invisible", in: "Барㅤсик", err: user.ErrInvisibleText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewPetName(tt.in)
			if !errors.Is(err, tt.err) || got.String() != tt.want {
				t.Fatalf("NewPetName(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestPetLevel(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		xp    int64
		level int
	}{{0, 1}, {99, 1}, {100, 2}, {299, 2}, {300, 3}, {1 << 40, user.MaxPetLevel}} {
		if got := user.PetLevel(tt.xp); got != tt.level {
			t.Errorf("PetLevel(%d) = %d, want %d", tt.xp, got, tt.level)
		}
	}

	if user.PetLevelExperience(3) != 300 {
		t.Fatalf("PetLevelExperience(3) = %d", user.PetLevelExperience(3))
	}
}

func TestNewPetShop(t *testing.T) {
	t.Parallel()

	def := user.DefaultPetShop()
	if _, err := user.NewPetShop(def.Prices(), def.Meal(), def.Win(), def.Fights()); err != nil {
		t.Fatalf("NewPetShop of the default shop: %v", err)
	}

	missing := def.Prices()
	delete(missing, user.PetDragon)
	if _, err := user.NewPetShop(missing, 5, 3, 10); !errors.Is(err, user.ErrPetShopPrice) {
		t.Fatalf("a kind without a price: %v", err)
	}
	if _, err := user.NewPetShop(def.Prices(), -1, 3, 10); !errors.Is(err, user.ErrPetShopPrice) {
		t.Fatalf("a meal below zero: %v", err)
	}
	if _, err := user.NewPetShop(def.Prices(), 5, 3, 0); !errors.Is(err, user.ErrPetShopFights) {
		t.Fatalf("no fights: %v", err)
	}
}

func TestPet_Care(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	p := user.NewPet(user.PetCat, "Барсик", now)

	// Hunger and boredom come with time, never below zero.
	later := now.Add(10 * time.Hour)
	if p.SatietyAt(later) != 40 || p.JoyAt(later) != 50 {
		t.Fatalf("feelings after 10 hours: %d, %d", p.SatietyAt(later), p.JoyAt(later))
	}
	if p.SatietyAt(now.Add(100*time.Hour)) != 0 {
		t.Fatal("satiety went below zero")
	}

	fed, err := p.Fed(later)
	if err != nil || fed.SatietyAt(later) != 80 || fed.Experience() != user.PetCareGain {
		t.Fatalf("Fed = %+v, %v", fed, err)
	}
	full, _ := fed.Fed(later)
	if _, err = full.Fed(later); !errors.Is(err, user.ErrPetFull) {
		t.Fatalf("Fed of a full pet: %v", err)
	}

	played, err := p.Played(later)
	if err != nil || played.JoyAt(later) != 80 || played.SatietyAt(later) != 35 {
		t.Fatalf("Played = %+v, %v", played, err)
	}
	if _, err = played.Played(later.Add(time.Minute)); !errors.Is(err, user.ErrPetTired) {
		t.Fatalf("Played again: %v", err)
	}
	if _, err = played.Played(later.Add(user.PetPlayEvery)); err != nil {
		t.Fatalf("Played after a while: %v", err)
	}
}

func TestPet_Fought(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	shop := user.DefaultPetShop()

	dragon := user.RestorePet(user.PetState{
		Kind: user.PetDragon, Name: "Смауг", Satiety: 100, Joy: 100, CountedAt: now,
		Experience: user.PetLevelExperience(10), Rating: 1000,
	})
	hamster := user.NewPet(user.PetHamster, "Хома", now)

	// With zero dice nobody dodges and the stronger dragon wins.
	a, d, bout, err := dragon.Fought(hamster, shop, now, nil)
	if err != nil || !bout.Won || bout.Rating != 16 || len(bout.Hits) == 0 {
		t.Fatalf("Fought = %+v, %v", bout, err)
	}
	last := bout.Hits[len(bout.Hits)-1]
	if last.DefenderHealth != 0 || !last.ByAttacker {
		t.Fatalf("the last hit: %+v", last)
	}
	if a.Rating() != 1016 || d.Rating() != 984 || a.Wins() != 1 || d.Losses() != 1 {
		t.Fatalf("after the fight: %d %d %d %d", a.Rating(), d.Rating(), a.Wins(), d.Losses())
	}
	if a.SatietyAt(now) != 90 || a.FightsAt(now) != 1 || d.Experience() != user.PetDefendGain {
		t.Fatalf("what the fight took: %d %d %d", a.SatietyAt(now), a.FightsAt(now), d.Experience())
	}
	if a.FightsAt(now.Add(24*time.Hour)) != 0 {
		t.Fatal("the fights of yesterday count today")
	}

	// The hamster loses to the dragon, and a hungry pet does not fight.
	if _, _, bout, err = hamster.Fought(dragon, shop, now, nil); err != nil || bout.Won || bout.Rating != -16 {
		t.Fatalf("the hamster fought: %+v, %v", bout, err)
	}
	hungry := user.RestorePet(user.PetState{Kind: user.PetCat, Satiety: 10, Joy: 100, CountedAt: now})
	if _, _, _, err = hungry.Fought(hamster, shop, now, nil); !errors.Is(err, user.ErrPetHungry) {
		t.Fatalf("a hungry pet fought: %v", err)
	}

	// Dice of the top numbers dodge every hit: the defender keeps it.
	dodges := make(user.PetDice, user.PetDiceCount)
	for i := range dodges {
		dodges[i] = 99
	}
	_, _, bout, _ = dragon.Fought(hamster, shop, now, dodges)
	if bout.Won || len(bout.Hits) != user.MaxPetTurns {
		t.Fatalf("a fight of dodges: %+v", bout)
	}

	equal := user.NewPet(user.PetCat, "Мурка", now)
	_, _, bout, _ = equal.Fought(user.NewPet(user.PetCat, "Пушок", now), shop, now, nil)
	if bout.Won != (bout.Hits[len(bout.Hits)-1].DefenderHealth == 0) {
		t.Fatalf("a fight of equals: %+v", bout)
	}

	tired := a
	for range shop.Fights() - 1 {
		fed, _ := tired.Fed(now)
		if fed != nil {
			tired = fed
		}
		if tired, _, _, err = tired.Fought(hamster, shop, now, nil); err != nil {
			t.Fatal(err)
		}
		if played, err := tired.Played(now); err == nil {
			tired = played
		}
		now = now.Add(user.PetPlayEvery)
	}
	if err = tired.CheckFightAt(shop, now); !errors.Is(err, user.ErrPetFightsOver) && !errors.Is(err, user.ErrPetHungry) && !errors.Is(err, user.ErrPetSad) {
		t.Fatalf("the fights of a day: %v", err)
	}
}
