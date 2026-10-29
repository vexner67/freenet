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

func TestPetService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")
	for _, id := range []uuid.UUID{ann, bob} {
		if _, err := w.users.Credit(ctx, id, 1000, 0, user.VoteEntry{}); err != nil {
			t.Fatal(err)
		}
	}
	for id, nick := range map[uuid.UUID]string{ann: "ann", bob: "bob"} {
		if _, err := w.userSvc.UpdateProfile(ctx, id, profile(t, nick), ""); err != nil {
			t.Fatal(err)
		}
	}

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	repo := inmem.NewPetRepository(w.users)
	pets := service.NewPetService(w.users, service.PetStores{Pets: repo, Keepers: repo, Battles: repo, Events: repo, Seasons: repo, Quests: repo, Market: repo}, w.users, inmem.TxManager{}, admins).
		WithRoll(func(int64) (int64, error) { return 0, nil })

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	if _, err = pets.Fight(ctx, ann, uuid.New(), "", "", now); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("Fight without a pet: %v", err)
	}

	// Ann adopts a dragon for 500 and a cat for 100, Bob a hamster for 50.
	care, err := pets.Adopt(ctx, ann, user.PetDragon, "Смауг", now)
	if err != nil || care.Balance != 500 {
		t.Fatalf("Adopt = %+v, %v", care, err)
	}
	dragon := care.Pet.ID()
	if care, err = pets.Adopt(ctx, ann, user.PetCat, "Мурка", now); err != nil || care.Balance != 400 {
		t.Fatalf("a second pet = %+v, %v", care, err)
	}
	cat := care.Pet.ID()
	care, err = pets.Adopt(ctx, bob, user.PetHamster, "Хома", now)
	if err != nil {
		t.Fatal(err)
	}
	hamsterID := care.Pet.ID()

	if own, err := pets.Pets(ctx, ann); err != nil || len(own) != 2 || own[0].ID() != dragon || own[1].ID() != cat {
		t.Fatalf("Pets = %v, %v", own, err)
	}
	if _, err = pets.Feed(ctx, ann, hamsterID, now); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("Feed of the pet of another: %v", err)
	}
	if _, err = pets.Fight(ctx, ann, dragon, "ann", "", now); !errors.Is(err, user.ErrPetFightSelf) {
		t.Fatalf("Fight of oneself: %v", err)
	}

	// Without a nickname the dragon fights the nearest pet — the hamster,
	// weaker — wins and brings 2 votes of 3: the win follows the power.
	fought, err := pets.Fight(ctx, ann, dragon, "", user.PetAttack, now)
	if err != nil || !fought.Fight.Bout.Won || fought.Fight.DefenderID != bob || fought.Balance != 402 || fought.Fight.Bout.Reward != 2 {
		t.Fatalf("Fight = %+v, %v", fought, err)
	}
	bobs, _, err := pets.UserPets(ctx, "bob", now)
	if err != nil || len(bobs) != 1 || bobs[0].Losses() != 1 || bobs[0].Rating() != user.PetStartRating-fought.Fight.Bout.Rating {
		t.Fatalf("the hamster after the fight: %+v, %v", bobs, err)
	}
	for _, id := range []uuid.UUID{ann, bob} {
		if fights, err := pets.Fights(ctx, id); err != nil || len(fights) != 1 {
			t.Fatalf("Fights = %v, %v", fights, err)
		}
	}

	// A meal costs 5 votes, a full pet takes none; a game is free.
	if care, err = pets.Feed(ctx, ann, dragon, now); err != nil || care.Balance != 397 {
		t.Fatalf("Feed = %+v, %v", care, err)
	}
	if _, err = pets.Feed(ctx, ann, dragon, now); !errors.Is(err, user.ErrPetFull) {
		t.Fatalf("Feed of a full pet: %v", err)
	}
	if _, err = pets.Play(ctx, ann, dragon, now); err != nil {
		t.Fatal(err)
	}
	if p, err := pets.Rename(ctx, ann, dragon, "Дрого"); err != nil || p.Name() != "Дрого" {
		t.Fatalf("Rename = %v, %v", p, err)
	}

	// The administrators set the shop; a released pet is gone.
	if _, err = pets.Settings(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Settings of a stranger: %v", err)
	}
	shop, _ := user.NewPetShop(user.DefaultPetShop().Prices(), 0, 0, 1)
	if err = pets.SetShop(ctx, admin, shop); err != nil {
		t.Fatal(err)
	}
	settings, err := pets.Settings(ctx, admin)
	if err != nil || settings.Stats.Pets != 3 || settings.Stats.Fights != 1 || settings.Stats.Spent != 655 || settings.Stats.Won != 2 {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}
	if _, err = pets.Fight(ctx, ann, dragon, "bob", "", now); !errors.Is(err, user.ErrPetFightsOver) {
		t.Fatalf("Fight past the fights of a day: %v", err)
	}

	// The board of pets puts the dragon first, each pet in its place.
	tops := service.NewTopService(inmem.NewTopRepository(w.users, repo))
	board, err := tops.Top(ctx, user.TopPets, now)
	if err != nil || len(board) != 3 || board[0].UserID != ann || board[0].PetName != "Дрого" {
		t.Fatalf("Top of pets = %+v, %v", board, err)
	}

	// Free pets cost nothing: Ann keeps up to ten.
	free := map[user.PetKind]int64{}
	for _, k := range user.PetKinds {
		free[k] = 0
	}
	if shop, err = user.NewPetShop(free, 0, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err = pets.SetShop(ctx, admin, shop); err != nil {
		t.Fatal(err)
	}
	for range user.MaxPets - 2 {
		if _, err = pets.Adopt(ctx, ann, user.PetCat, "Кот", now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pets.Adopt(ctx, ann, user.PetCat, "Кот", now); !errors.Is(err, user.ErrTooManyPets) {
		t.Fatalf("an eleventh pet: %v", err)
	}

	// A released hamster leaves a shard.
	if keeper, err := pets.Release(ctx, bob, hamsterID); err != nil || keeper.Shards() != 1 {
		t.Fatalf("Release = %v, %v", keeper, err)
	}
	if bobs, _, err = pets.UserPets(ctx, "bob", now); err != nil || len(bobs) != 0 {
		t.Fatalf("a released pet: %v, %v", bobs, err)
	}
	if _, err = pets.Release(ctx, bob, hamsterID); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("Release twice: %v", err)
	}
	if _, err = pets.Fight(ctx, ann, dragon, "bob", "", now.Add(24*time.Hour)); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("Fight of a page without pets: %v", err)
	}
}

func TestTopService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann, bob := w.person(t, "ann"), w.person(t, "bob")
	tops := service.NewTopService(inmem.NewTopRepository(w.users, inmem.NewPetRepository(w.users)))
	now := time.Now()

	if _, err := w.users.Credit(ctx, ann, 300, 0, user.Entry(user.VoteMine, uuid.Nil, "stone")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.users.Credit(ctx, bob, 900, 0, user.Entry(user.VoteCasino, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.users.Spend(ctx, bob, 1000, user.Entry(user.VoteCasino, uuid.Nil, "")); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Spend: %v", err)
	}
	if _, err := w.users.Spend(ctx, bob, 500, user.Entry(user.VoteRocket, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}

	rich, err := tops.Top(ctx, user.TopRich, now)
	if err != nil || len(rich) != 2 || rich[0].UserID != bob || rich[0].Value != 400 {
		t.Fatalf("Top of the rich = %+v, %v", rich, err)
	}
	miners, err := tops.Top(ctx, user.TopMiners, now)
	if err != nil || len(miners) != 1 || miners[0].UserID != ann || miners[0].Value != 300 {
		t.Fatalf("Top of the miners = %+v, %v", miners, err)
	}
	casino, err := tops.Top(ctx, user.TopCasino, now)
	if err != nil || len(casino) != 1 || casino[0].Value != 400 {
		t.Fatalf("Top of the casino = %+v, %v", casino, err)
	}
	if business, err := tops.Top(ctx, user.TopBusiness, now); err != nil || len(business) != 0 {
		t.Fatalf("Top of business = %+v, %v", business, err)
	}
}

func TestGuestService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann, bob, eve := w.person(t, "ann"), w.person(t, "bob"), w.person(t, "eve")
	for id, nick := range map[uuid.UUID]string{ann: "ann", bob: "bob", eve: "eve"} {
		if _, err := w.userSvc.UpdateProfile(ctx, id, profile(t, nick), ""); err != nil {
			t.Fatal(err)
		}
	}

	guests := service.NewGuestService(w.users, inmem.NewGuestRepository(w.users))
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	if err := guests.Visit(ctx, bob, "nobody", now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Visit of nobody: %v", err)
	}
	for i, id := range []uuid.UUID{bob, eve, ann} {
		if err := guests.Visit(ctx, id, "ann", now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// Bob comes again: one guest, the latest visit.
	if err := guests.Visit(ctx, bob, "ann", now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if n, err := guests.NewGuests(ctx, ann, now.Add(time.Hour)); err != nil || n != 2 {
		t.Fatalf("NewGuests = %d, %v", n, err)
	}

	page, err := guests.Guests(ctx, ann, "", 1, now.Add(time.Hour))
	if err != nil || len(page.Guests) != 1 || page.Guests[0].GuestID != bob || page.NextCursor == "" || !page.SeenAt.IsZero() {
		t.Fatalf("Guests = %+v, %v", page, err)
	}
	next, err := guests.Guests(ctx, ann, page.NextCursor, 1, now.Add(time.Hour))
	if err != nil || len(next.Guests) != 1 || next.Guests[0].GuestID != eve || next.NextCursor != "" {
		t.Fatalf("the next page = %+v, %v", next, err)
	}
	if n, _ := guests.NewGuests(ctx, ann, now.Add(time.Hour)); n != 0 {
		t.Fatalf("NewGuests after a look = %d", n)
	}

	// A month later the visits are gone.
	if page, _ = guests.Guests(ctx, ann, "", 10, now.Add(user.GuestsFor+time.Hour)); len(page.Guests) != 0 {
		t.Fatalf("old guests: %+v", page)
	}
	if _, err = guests.Guests(ctx, ann, "bad", 10, now); !errors.Is(err, service.ErrCursor) {
		t.Fatalf("a bad cursor: %v", err)
	}
}

func TestPetCasesAndClothes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann := w.person(t, "ann")
	if _, err := w.users.Credit(ctx, ann, 2000, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	repo := inmem.NewPetRepository(w.users)
	// The rolls fall high: a legendary pet, the second of its kinds.
	pets := service.NewPetService(w.users, service.PetStores{Pets: repo, Keepers: repo, Battles: repo, Events: repo, Seasons: repo, Quests: repo, Market: repo}, w.users, inmem.TxManager{}, user.Admins{}).
		WithRoll(func(n int64) (int64, error) { return n - 1, nil })
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	drop, err := pets.OpenCase(ctx, ann, user.PetCaseBronze, false, now)
	if err != nil || drop.Keeper.Drop().Rarity() != user.PetLegendary || drop.Balance != 1850+questPaid(ann, now, user.PetQuestCase) {
		t.Fatalf("OpenCase = %+v, %v", drop, err)
	}
	if _, err = pets.OpenCase(ctx, ann, user.PetCaseBronze, false, now); !errors.Is(err, user.ErrPetDropWaiting) {
		t.Fatalf("a case with a pet waiting: %v", err)
	}

	// The pet is left for 20 shards, and the next one taken.
	keeper, err := pets.LeaveDrop(ctx, ann)
	if err != nil || keeper.Shards() != 20 || keeper.Drop() != "" {
		t.Fatalf("LeaveDrop = %+v, %v", keeper, err)
	}
	if _, err = pets.TakeDrop(ctx, ann, "Находка", now); !errors.Is(err, user.ErrNoPetDrop) {
		t.Fatalf("TakeDrop without a pet: %v", err)
	}
	if drop, err = pets.OpenCase(ctx, ann, user.PetCaseBronze, true, now); err != nil || drop.Keeper.Shards() != 10 || drop.Balance != 0 {
		t.Fatalf("OpenCase for shards = %+v, %v", drop, err)
	}
	taken, err := pets.TakeDrop(ctx, ann, "Находка", now)
	if err != nil || taken.Kind() != drop.Keeper.Drop() {
		t.Fatalf("TakeDrop = %v, %v", taken, err)
	}
	if keeper, err = pets.Keeper(ctx, ann); err != nil || keeper.Drop() != "" || keeper.CasesAt(now) != 2 {
		t.Fatalf("Keeper = %+v, %v", keeper, err)
	}
	found := taken.ID()

	care, err := pets.BuyItem(ctx, ann, found, "crown")
	if err != nil || care.Balance != 350+questPaid(ann, now, user.PetQuestCase) || len(care.Pet.Outfit()) != 1 {
		t.Fatalf("BuyItem = %+v, %v", care, err)
	}
	if _, err = pets.BuyItem(ctx, ann, found, "crown"); !errors.Is(err, user.ErrPetItemOwned) {
		t.Fatalf("buying the crown again: %v", err)
	}
	if _, err = pets.BuyItem(ctx, ann, found, "cape"); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("buying without votes: %v", err)
	}
	p, err := pets.Dress(ctx, ann, found, "crown", false)
	if err != nil || len(p.Outfit()) != 0 || len(p.Wardrobe()) != 1 {
		t.Fatalf("Dress = %v, %v", p, err)
	}
	if _, err = pets.Dress(ctx, ann, found, "cape", true); !errors.Is(err, user.ErrPetItemMissing) {
		t.Fatalf("wearing what was not bought: %v", err)
	}
	if kept, err := pets.Pets(ctx, ann); err != nil || len(kept) != 1 || len(kept[0].Wardrobe()) != 1 {
		t.Fatalf("Pets = %v, %v", kept, err)
	}

	// A case is not opened without votes.
	if _, err = pets.OpenCase(ctx, ann, user.PetCaseGold, false, now); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("OpenCase without votes: %v", err)
	}
}

func TestPetBattles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann, bob := w.person(t, "ann"), w.person(t, "bob")
	for id, nick := range map[uuid.UUID]string{ann: "ann", bob: "bob"} {
		if _, err := w.users.Credit(ctx, id, 1000, 0, user.VoteEntry{}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.userSvc.UpdateProfile(ctx, id, profile(t, nick), ""); err != nil {
			t.Fatal(err)
		}
	}

	repo := inmem.NewPetRepository(w.users)
	pets := service.NewPetService(w.users, service.PetStores{Pets: repo, Keepers: repo, Battles: repo, Events: repo, Seasons: repo, Quests: repo, Market: repo}, w.users, inmem.TxManager{}, user.Admins{}).
		WithRoll(func(int64) (int64, error) { return 0, nil })
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	care, err := pets.Adopt(ctx, ann, user.PetDog, "Шарик", now)
	if err != nil {
		t.Fatal(err)
	}
	dog := care.Pet.ID()
	if care, err = pets.Adopt(ctx, bob, user.PetCat, "Мурка", now); err != nil {
		t.Fatal(err)
	}
	cat := care.Pet.ID()

	// Bob's cat defends in agility.
	if p, err := pets.SetStance(ctx, bob, cat, user.PetAgility); err != nil || p.Stance() != user.PetAgility {
		t.Fatalf("SetStance = %v, %v", p, err)
	}

	if _, err = pets.MoveBattle(ctx, ann, dog, user.PetStrike, now); !errors.Is(err, user.ErrNoPetBattle) {
		t.Fatalf("a move without a fight: %v", err)
	}

	turn, err := pets.StartBattle(ctx, ann, dog, "bob", user.PetAttack, now)
	if err != nil || turn.Battle.DefenderOwner() != bob || turn.Battle.Defender().Fighter.Stance != user.PetAgility || turn.Pet.Losses() != 1 {
		t.Fatalf("StartBattle = %+v, %v", turn, err)
	}
	if _, err = pets.StartBattle(ctx, ann, dog, "bob", user.PetAttack, now); !errors.Is(err, user.ErrPetInBattle) {
		t.Fatalf("a second fight: %v", err)
	}
	if _, err = pets.Fight(ctx, ann, dog, "bob", "", now); !errors.Is(err, user.ErrPetInBattle) {
		t.Fatalf("a quick fight in a fight: %v", err)
	}
	if battles, err := pets.Battles(ctx, ann); err != nil || len(battles) != 1 {
		t.Fatalf("Battles = %v, %v", battles, err)
	}

	if turn, err = pets.MoveBattle(ctx, ann, dog, user.PetGuard, now); err != nil || turn.Battle.Round() != 1 || turn.Fought != nil {
		t.Fatalf("MoveBattle = %+v, %v", turn, err)
	}
	if turn, err = pets.MoveBattle(ctx, ann, dog, user.PetAuto, now); err != nil || turn.Fought == nil || !turn.Battle.Done() {
		t.Fatalf("the rest of the fight = %+v, %v", turn, err)
	}
	if fights, err := pets.Fights(ctx, bob); err != nil || len(fights) != 1 || fights[0].Bout.Won != turn.Fought.Fight.Bout.Won {
		t.Fatalf("Fights = %v, %v", fights, err)
	}
	if battles, err := pets.Battles(ctx, ann); err != nil || len(battles) != 0 {
		t.Fatalf("Battles after the end = %v, %v", battles, err)
	}

	// A fight left halfway is lost, and a released pet takes its fight.
	if _, err = pets.StartBattle(ctx, ann, dog, "bob", user.PetAttack, now); err != nil {
		t.Fatal(err)
	}
	if _, err = pets.Release(ctx, ann, dog); err != nil {
		t.Fatal(err)
	}
	if battles, err := pets.Battles(ctx, ann); err != nil || len(battles) != 0 {
		t.Fatalf("the fight of a released pet = %v, %v", battles, err)
	}
}

// questPaid is what a task of kind done once pays userID on the day of
// now: the tasks are drawn for each person and day.
func questPaid(userID uuid.UUID, now time.Time, kind user.PetQuestKind) int64 {
	for _, q := range user.PetQuestsOf(userID, user.BonusDay(now)) {
		if q.Kind == kind && q.Goal == 1 {
			return user.PetQuestReward
		}
	}

	return 0
}
