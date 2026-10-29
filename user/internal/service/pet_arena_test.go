package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPetArena(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann, bob := w.person(t, "ann"), w.person(t, "bob")
	for id, nick := range map[uuid.UUID]string{ann: "ann", bob: "bob"} {
		if _, err := w.users.Credit(ctx, id, 2000, 0, user.VoteEntry{}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.userSvc.UpdateProfile(ctx, id, profile(t, nick), ""); err != nil {
			t.Fatal(err)
		}
	}

	repo := inmem.NewPetRepository(w.users)
	pets := service.NewPetService(w.users, service.PetStores{Pets: repo, Keepers: repo, Battles: repo, Events: repo, Seasons: repo, Quests: repo, Market: repo}, w.users, inmem.TxManager{}, user.Admins{}).
		WithRoll(func(int64) (int64, error) { return 0, nil })
	votes := func(id uuid.UUID) int64 {
		u, err := w.users.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}

		return u.Wallet().Votes
	}

	// Monday, 26 October.
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	care, err := pets.Adopt(ctx, ann, user.PetDragon, "Смауг", now)
	if err != nil {
		t.Fatal(err)
	}
	dragon := care.Pet.ID()
	if care, err = pets.Adopt(ctx, ann, user.PetCat, "Мурка", now); err != nil {
		t.Fatal(err)
	}
	cat := care.Pet.ID()
	if care, err = pets.Adopt(ctx, bob, user.PetHamster, "Хома", now); err != nil {
		t.Fatal(err)
	}
	hamster := care.Pet.ID()

	// The first look at the arena closes the last season with no places.
	arena, err := pets.Arena(ctx, ann, now)
	if err != nil || arena.Boss.Health() != user.PetBossHealth || !arena.Tournament.Open(now) || arena.Last != nil ||
		len(arena.Seasons) != 1 || len(arena.Seasons[0].Places) != 0 || len(arena.Top) != 2 || arena.Quests == nil {
		t.Fatalf("Arena = %+v, %v", arena, err)
	}

	// The dragon hits the boss and rests.
	done, err := pets.RunArena(ctx, service.PetArenaAct{Action: service.PetBossHit, UserID: ann, PetID: dragon, Now: now})
	if err != nil || done.Bout == nil {
		t.Fatalf("boss = %+v, %v", done, err)
	}
	if arena, err = pets.Arena(ctx, ann, now); err != nil || arena.Boss.Health() >= user.PetBossHealth || arena.Boss.HitOf(ann).Hits != 1 {
		t.Fatalf("the boss after a hit = %+v, %v", arena.Boss, err)
	}
	if _, err = pets.RunArena(ctx, service.PetArenaAct{Action: service.PetBossHit, UserID: ann, PetID: dragon, Now: now}); !errors.Is(err, user.ErrPetBossRest) {
		t.Fatalf("a hit at rest: %v", err)
	}
	if k, err := pets.Keeper(ctx, ann); err != nil || k.Shards() < user.PetBossShards {
		t.Fatalf("shards of a hit = %v, %v", k, err)
	}

	// Both enter the tournament, one pet each; Bob bets on the dragon.
	act := func(a service.PetArenaAct) error {
		a.Now = now
		_, err := pets.RunArena(ctx, a)

		return err
	}
	if err = act(service.PetArenaAct{Action: service.PetJoin, UserID: ann, PetID: dragon}); err != nil {
		t.Fatal(err)
	}
	if err = act(service.PetArenaAct{Action: service.PetJoin, UserID: ann, PetID: cat}); !errors.Is(err, user.ErrPetTournamentJoined) {
		t.Fatalf("a second pet: %v", err)
	}
	if err = act(service.PetArenaAct{Action: service.PetJoin, UserID: bob, PetID: hamster}); err != nil {
		t.Fatal(err)
	}
	before := votes(bob)
	if err = act(service.PetArenaAct{Action: service.PetBetOn, UserID: bob, PetID: dragon, Votes: 100}); err != nil || votes(bob) != before-100 {
		t.Fatalf("bet = %v, votes %d", err, votes(bob))
	}
	if err = act(service.PetArenaAct{Action: service.PetBetOn, UserID: bob, PetID: dragon, Votes: 100}); !errors.Is(err, user.ErrPetBetPlaced) {
		t.Fatalf("a second bet: %v", err)
	}

	// Sunday after 20:00 in Moscow the tournament is held and paid, and
	// on the first of November October is closed too.
	sunday := time.Date(2026, 11, 1, 18, 0, 0, 0, time.UTC)
	total := votes(ann) + votes(bob)
	if arena, err = pets.Arena(ctx, ann, sunday); err != nil || !arena.Tournament.Held() {
		t.Fatalf("Arena on Sunday = %+v, %v", arena.Tournament, err)
	}
	winner, ok := arena.Tournament.Winner()
	if !ok {
		t.Fatal("no winner")
	}
	paid := user.PetTournamentPrizes[0] + user.PetTournamentPrizes[1] + user.PetSeasonPrizes[0] + user.PetSeasonPrizes[1] + 100
	if winner.PetID == dragon {
		paid -= 100 * user.PetBetCut / 100
	}
	if got := votes(ann) + votes(bob); got != total+paid {
		t.Fatalf("votes after the tournament = %d, want %d", got, total+paid)
	}
	if len(arena.Seasons) != 2 || !arena.Seasons[0].Season.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || len(arena.Seasons[0].Places) != 2 {
		t.Fatalf("Seasons = %+v", arena.Seasons)
	}
	champion, err := pets.Arena(ctx, winner.Owner, sunday)
	if err != nil || !hasAchievement(champion.Achievements, user.PetChampion) || !hasAchievement(champion.Achievements, user.PetSeasonBest) {
		t.Fatalf("Achievements of the winner = %+v, %v", champion.Achievements, err)
	}

	// A week after getting it, Ann sells the cat to Bob; his pets go by
	// their IDs, the cat first.
	later := now.Add(user.PetResaleAfter)
	if err = act(service.PetArenaAct{Action: service.PetSell, UserID: ann, PetID: cat, Votes: 100}); !errors.Is(err, user.ErrPetResale) {
		t.Fatalf("a sale too soon: %v", err)
	}
	sell := service.PetArenaAct{Action: service.PetSell, UserID: ann, PetID: cat, Votes: 10, Now: later}
	if _, err = pets.RunArena(ctx, sell); !errors.Is(err, user.ErrPetSalePrice) {
		t.Fatalf("a sale too cheap: %v", err)
	}
	sell.Votes = 100
	if _, err = pets.RunArena(ctx, sell); err != nil {
		t.Fatal(err)
	}
	if arena, err = pets.Arena(ctx, bob, later); err != nil || len(arena.Market) != 1 || arena.Market[0].Pet.ID() != cat {
		t.Fatalf("Market = %+v, %v", arena.Market, err)
	}
	if _, err = pets.RunArena(ctx, service.PetArenaAct{Action: service.PetBuy, UserID: ann, PetID: cat, Now: later}); !errors.Is(err, user.ErrPetOwnSale) {
		t.Fatalf("buying one's own: %v", err)
	}
	annVotes, bobVotes := votes(ann), votes(bob)
	if _, err = pets.RunArena(ctx, service.PetArenaAct{Action: service.PetBuy, UserID: bob, PetID: cat, Now: later}); err != nil {
		t.Fatal(err)
	}
	if votes(ann) != annVotes+90 || votes(bob) != bobVotes-100 {
		t.Fatalf("votes after the sale = %d, %d", votes(ann), votes(bob))
	}
	if own, err := pets.Pets(ctx, bob); err != nil || len(own) != 2 || own[0].ID() != cat || own[0].SalePrice() != 0 {
		t.Fatalf("Bob's pets = %v, %v", own, err)
	}

	if _, err = pets.RunArena(ctx, service.PetArenaAct{Action: "dance", UserID: ann, Now: later}); !errors.Is(err, service.ErrPetArenaAction) {
		t.Fatalf("an unknown action: %v", err)
	}
}

func hasAchievement(got []user.PetAchievementGot, a user.PetAchievement) bool {
	for _, g := range got {
		if g.Achievement == a {
			return true
		}
	}

	return false
}
