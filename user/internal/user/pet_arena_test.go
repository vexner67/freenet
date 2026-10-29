package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

// arenaNow is a Wednesday noon of Moscow.
var arenaNow = time.Date(2027, 3, 3, 9, 0, 0, 0, time.UTC)

func TestPetWeekAndSeason(t *testing.T) {
	t.Parallel()

	if w := user.PetWeek(arenaNow); !w.Equal(time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("week %v", w)
	}
	// Sunday 23:30 of Moscow is still the week before Monday.
	sunday := time.Date(2027, 3, 7, 20, 30, 0, 0, time.UTC)
	if w := user.PetWeek(sunday); !w.Equal(time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("week of Sunday night %v", w)
	}
	if start := user.PetTournamentStart(user.PetWeek(arenaNow)); !start.Equal(time.Date(2027, 3, 7, 17, 0, 0, 0, time.UTC)) {
		t.Errorf("the tournament starts %v", start)
	}
	if s := user.PetSeason(arenaNow); !s.Equal(time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("season %v", s)
	}
	if end := user.PetSeasonEnd(arenaNow); !end.Equal(time.Date(2027, 3, 31, 21, 0, 0, 0, time.UTC)) {
		t.Errorf("season ends %v", end)
	}
	for _, tc := range []struct{ in, want int }{{1000, 1000}, {1400, 1200}, {800, 900}} {
		if got := user.SeasonRating(tc.in); got != tc.want {
			t.Errorf("SeasonRating(%d) = %d", tc.in, got)
		}
	}
}

func arenaPet(kind user.PetKind, experience int64) *user.Pet {
	s := user.NewPet(kind, "Тест", arenaNow.Add(-30*24*time.Hour)).State()
	s.CountedAt, s.Experience = arenaNow, experience

	return user.RestorePet(s)
}

func TestPetBoss(t *testing.T) {
	t.Parallel()

	shop := user.DefaultPetShop()
	ann, bob := uuid.New(), uuid.New()
	boss := user.NewPetBoss(arenaNow)
	if boss.Health() != user.PetBossHealth || boss.Down() {
		t.Fatalf("a new boss: %+v", boss.State())
	}

	p := arenaPet(user.PetWolf, 20_000)
	b, hit, bout, err := boss.Hit(ann, p, shop, arenaNow, nil)
	if err != nil || len(bout.Hits) == 0 {
		t.Fatalf("hit: %v", err)
	}
	damage := boss.Health() - b.Health()
	if damage <= 0 || b.HitOf(ann).Damage != damage || b.HitOf(ann).Hits != 1 {
		t.Errorf("damage %d, kept %+v", damage, b.HitOf(ann))
	}
	if _, _, _, err = b.Hit(ann, hit, shop, arenaNow.Add(time.Hour), nil); !errors.Is(err, user.ErrPetBossRest) {
		t.Errorf("hitting again within the rest: %v", err)
	}

	// The boss falls to the hits; the last one finishes it.
	at := arenaNow
	for i := 0; !b.Down() && i < 10_000; i++ {
		at = at.Add(user.PetBossRest)
		who := ann
		if i%2 == 1 {
			who = bob
		}
		fresh := arenaPet(user.PetWolf, 20_000)
		s := fresh.State()
		s.CountedAt = at
		if b, _, _, err = b.Hit(who, user.RestorePet(s), shop, at, nil); err != nil {
			t.Fatal(err)
		}
	}
	if !b.Down() || b.KilledBy() == uuid.Nil {
		t.Fatalf("the boss stands: %+v", b.Health())
	}
	if _, _, _, err = b.Hit(ann, p, shop, at.Add(user.PetBossRest), nil); !errors.Is(err, user.ErrPetBossDown) {
		t.Errorf("hitting a beaten boss: %v", err)
	}

	var paid int64
	for _, pay := range b.Prizes() {
		paid += pay.Votes
	}
	if paid < user.PetBossPrize || paid > user.PetBossPrize+user.PetBossLastHit {
		t.Errorf("the boss paid %d", paid)
	}
}

func TestPetTournament(t *testing.T) {
	t.Parallel()

	tour := user.NewPetTournament(arenaNow)
	pets := map[uuid.UUID]*user.Pet{}
	owners := make([]uuid.UUID, 5)
	var err error
	for i := range owners {
		owners[i] = uuid.New()
		p := arenaPet(user.PetCat, int64(i)*2_000)
		pets[p.ID()] = p
		if tour, err = tour.Joined(owners[i], p, arenaNow); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tour.Joined(owners[0], arenaPet(user.PetDog, 0), arenaNow); !errors.Is(err, user.ErrPetTournamentJoined) {
		t.Errorf("a second pet: %v", err)
	}

	strongest := tour.Entrants()[4].PetID
	better, punter := uuid.New(), uuid.New()
	if tour, err = tour.Bet(better, strongest, 100, arenaNow); err != nil {
		t.Fatal(err)
	}
	if tour, err = tour.Bet(punter, tour.Entrants()[0].PetID, 300, arenaNow); err != nil {
		t.Fatal(err)
	}
	if _, err = tour.Bet(better, strongest, 100, arenaNow); !errors.Is(err, user.ErrPetBetPlaced) {
		t.Errorf("a second bet: %v", err)
	}
	if _, err = tour.Bet(uuid.New(), strongest, 5, arenaNow); !errors.Is(err, user.ErrPetBet) {
		t.Errorf("a bet too small: %v", err)
	}

	start := tour.StartsAt()
	if _, err = tour.Joined(uuid.New(), arenaPet(user.PetDog, 0), start); !errors.Is(err, user.ErrPetTournamentClosed) {
		t.Errorf("joining after the start: %v", err)
	}
	if !tour.Due(start) || tour.Due(start.Add(-time.Minute)) {
		t.Error("the tournament is due at its start")
	}

	dice := func() user.PetDice { return nil }
	played := tour.Played(pets, start, dice)
	if len(played.Rounds()) != 3 || !played.Held() {
		t.Fatalf("five pets fight three rounds: %d", len(played.Rounds()))
	}
	w, ok := played.Winner()
	if !ok || len(played.Places()) != 4 || len(played.Prizes()) != 4 || played.Places()[0] != w {
		t.Fatalf("winner %+v, places %+v", w, played.Places())
	}

	paid, cut := played.Winnings()
	var total int64
	for _, p := range paid {
		total += p.Votes
	}
	if total+cut != 400 {
		t.Errorf("the bets paid %d and cut %d of 400", total, cut)
	}

	// Alone, a pet has nobody to fight: the bets go back.
	lone := user.NewPetTournament(arenaNow)
	p := arenaPet(user.PetCat, 0)
	lone, _ = lone.Joined(owners[0], p, arenaNow)
	lone, _ = lone.Bet(better, p.ID(), 50, arenaNow)
	lone = lone.Played(map[uuid.UUID]*user.Pet{p.ID(): p}, start, dice)
	if _, ok := lone.Winner(); ok {
		t.Error("a tournament of one has a winner")
	}
	if paid, cut := lone.Winnings(); len(paid) != 1 || paid[0].Votes != 50 || cut != 0 {
		t.Errorf("the lone bet back: %+v, %d", paid, cut)
	}
}

func TestPetMarket(t *testing.T) {
	t.Parallel()

	later := arenaNow.Add(user.PetResaleAfter)
	fresh := user.NewPet(user.PetCat, "Мурка", arenaNow).State()
	fresh.CountedAt = later
	p := user.RestorePet(fresh)
	if _, err := p.ListedFor(100, arenaNow); !errors.Is(err, user.ErrPetResale) {
		t.Errorf("selling a new pet: %v", err)
	}
	if _, err := p.ListedFor(user.PetCommon.MinPrice()-1, later); !errors.Is(err, user.ErrPetSalePrice) {
		t.Errorf("selling too cheap: %v", err)
	}
	listed, err := p.ListedFor(100, later)
	if err != nil || listed.SalePrice() != 100 {
		t.Fatalf("list: %v", err)
	}
	if err = listed.CheckFightAt(user.DefaultPetShop(), later); !errors.Is(err, user.ErrPetOnSale) {
		t.Errorf("a pet for sale fights: %v", err)
	}
	sold, err := listed.SoldAt(later)
	if err != nil || sold.SalePrice() != 0 || !sold.OwnedSince().Equal(later) || !sold.SaleAt().Equal(later.Add(user.PetResaleAfter)) {
		t.Errorf("sold: %v", err)
	}
	if _, err = p.SoldAt(later); !errors.Is(err, user.ErrPetNotOnSale) {
		t.Errorf("buying a pet not for sale: %v", err)
	}
	if user.PetSaleFee(1000) != 100 {
		t.Errorf("fee %d", user.PetSaleFee(1000))
	}
}

func TestPetQuests(t *testing.T) {
	t.Parallel()

	who := uuid.New()
	today := user.PetQuestsOf(who, user.BonusDay(arenaNow))
	if len(today) != user.PetQuestsADay || today[0] == today[1] {
		t.Fatalf("quests %+v", today)
	}

	q := user.RestorePetQuests(who, user.PetQuestsState{})
	var prize user.PetQuestPrize
	var votes int64
	for _, task := range today {
		for range task.Goal {
			q, prize = q.Did(task.Kind, 1, arenaNow)
			votes += prize.Votes
		}
	}
	if prize.Shards != user.PetQuestShards || q.StreakAt(arenaNow) != 1 {
		t.Errorf("all done: %+v, streak %d", prize, q.StreakAt(arenaNow))
	}
	if votes != 3*user.PetQuestReward+user.PetStreakReward {
		t.Errorf("the day paid %d", votes)
	}

	// Done again, a task pays nothing; a task not of the day neither.
	if _, again := q.Did(today[0].Kind, 1, arenaNow); again.Votes != 0 {
		t.Errorf("a task paid twice: %+v", again)
	}

	tomorrow := arenaNow.Add(24 * time.Hour)
	if q.ProgressAt(today[0].Kind, tomorrow) != 0 || q.StreakAt(tomorrow) != 1 || q.StreakAt(tomorrow.Add(24*time.Hour)) != 0 {
		t.Error("the day after starts anew and keeps the streak a day")
	}
}

func TestEarnedAchievements(t *testing.T) {
	t.Parallel()

	phoenix := arenaPet(user.PetPhoenix, 0)
	got := user.EarnedAchievements([]*user.Pet{phoenix}, 2_000_000, 7)
	for _, a := range []user.PetAchievement{user.PetLegend, user.PetFirstPhoenix, user.PetWeekStreak, user.PetMillionaire} {
		found := false
		for _, g := range got {
			found = found || g == a
		}
		if !found {
			t.Errorf("%s not earned: %v", a, got)
		}
	}
	if len(user.EarnedAchievements(nil, 0, 0)) != 0 {
		t.Error("nothing earns a badge")
	}
}
