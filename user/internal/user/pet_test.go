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

func TestPetCases(t *testing.T) {
	t.Parallel()

	for _, c := range user.PetCases {
		sum := 0
		for _, odds := range c.Odds() {
			sum += odds
		}
		if sum != user.PetCaseOdds {
			t.Errorf("the odds of %s add up to %d", c, sum)
		}
	}
	for _, r := range user.PetRarities {
		if len(user.PetKindsOf(r)) == 0 {
			t.Errorf("no kind is %s", r)
		}
	}

	tests := []struct {
		c      user.PetCase
		rarity int64
		kind   int64
		want   user.PetKind
	}{
		{user.PetCaseBronze, 0, 0, user.PetCat},
		{user.PetCaseBronze, 719, 4, user.PetTurtle},
		{user.PetCaseBronze, 720, 0, user.PetFox},
		{user.PetCaseBronze, 998, 1, user.PetPhoenix},
		{user.PetCaseGold, 0, 1, user.PetOwl},
		{user.PetCaseGold, 970, 2, user.PetUnicorn},
	}
	for _, tt := range tests {
		if got := tt.c.Draw(tt.rarity, tt.kind); got != tt.want {
			t.Errorf("%s.Draw(%d, %d) = %s, want %s", tt.c, tt.rarity, tt.kind, got, tt.want)
		}
	}

	if _, err := user.NewPetCase("diamond"); !errors.Is(err, user.ErrUnknownPetCase) {
		t.Errorf("NewPetCase of an unknown case: %v", err)
	}

	shop, err := user.DefaultPetShop().WithCases(map[user.PetCase]int64{user.PetCaseBronze: 1, user.PetCaseSilver: 2, user.PetCaseGold: 3})
	if err != nil || shop.CasePrice(user.PetCaseGold) != 3 {
		t.Fatalf("WithCases = %v, %v", shop.Cases(), err)
	}
	if _, err = shop.WithCases(map[user.PetCase]int64{user.PetCaseBronze: 1}); !errors.Is(err, user.ErrPetShopPrice) {
		t.Errorf("WithCases without every case: %v", err)
	}
	if _, err = shop.Price(user.PetPhoenix); !errors.Is(err, user.ErrNotInPetShop) {
		t.Errorf("the price of a phoenix: %v", err)
	}
}

func TestPetClothes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	p := user.NewPet(user.PetDog, "Шарик", now)
	base := p.Stats()

	if _, err := user.NewPetItem("tie"); !errors.Is(err, user.ErrUnknownPetItem) {
		t.Errorf("NewPetItem of an unknown thing: %v", err)
	}
	if _, err := p.Dressed("cap", true); !errors.Is(err, user.ErrPetItemMissing) {
		t.Errorf("wearing what was not bought: %v", err)
	}

	// The cap is put on as it is bought; the helmet takes its place.
	capped, err := p.Bought("cap")
	if err != nil || capped.Stats().Speed != base.Speed+2 || len(capped.Outfit()) != 1 {
		t.Fatalf("Bought = %+v, %v", capped.Stats(), err)
	}
	if _, err = capped.Bought("cap"); !errors.Is(err, user.ErrPetItemOwned) {
		t.Errorf("buying the cap twice: %v", err)
	}
	helmet, err := capped.Bought("helmet")
	if err != nil || len(helmet.Outfit()) != 1 || helmet.Outfit()[0] != "helmet" || len(helmet.Wardrobe()) != 2 {
		t.Fatalf("a second hat: %v, %v, %v", helmet.Outfit(), helmet.Wardrobe(), err)
	}
	scarf, err := helmet.Bought("scarf")
	if err != nil || scarf.ClothesStats() != (user.PetStats{Defense: 3, Health: 12}) {
		t.Fatalf("a hat and a scarf: %+v, %v", scarf.ClothesStats(), err)
	}
	if scarf.Stats() != scarf.BreedStats().Add(scarf.LevelStats()).Add(scarf.ClothesStats()) {
		t.Errorf("the stats do not add up")
	}

	bare, err := scarf.Dressed("helmet", false)
	if err != nil || len(bare.Outfit()) != 1 || bare.Outfit()[0] != "scarf" {
		t.Fatalf("taking the helmet off: %v, %v", bare.Outfit(), err)
	}
	// The pet before stays as it was.
	if len(scarf.Outfit()) != 2 || len(p.Wardrobe()) != 0 {
		t.Errorf("a change leaked: %v, %v", scarf.Outfit(), p.Wardrobe())
	}

	// A fight remembers the clothes.
	rival := user.NewPet(user.PetCat, "Мурка", now)
	_, _, bout, err := scarf.Fought(rival, user.PetAttack, user.DefaultPetShop(), now, nil)
	if err != nil || len(bout.Attacker.Outfit) != 2 || bout.Attacker.Health != scarf.Stats().Health {
		t.Fatalf("Fought = %+v, %v", bout.Attacker, err)
	}
}
