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
	pets := service.NewPetService(w.users, repo, w.users, inmem.TxManager{}, admins).
		WithRoll(func(int64) (int64, error) { return 0, nil })

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	if _, err = pets.Fight(ctx, ann, uuid.New(), "", now); !errors.Is(err, user.ErrPetNotFound) {
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
	if _, err = pets.Fight(ctx, ann, dragon, "ann", now); !errors.Is(err, user.ErrPetFightSelf) {
		t.Fatalf("Fight of oneself: %v", err)
	}

	// Without a nickname the dragon fights the nearest pet — the hamster —
	// wins and brings 3 votes.
	fought, err := pets.Fight(ctx, ann, dragon, "", now)
	if err != nil || !fought.Fight.Bout.Won || fought.Fight.DefenderID != bob || fought.Balance != 403 {
		t.Fatalf("Fight = %+v, %v", fought, err)
	}
	bobs, _, err := pets.UserPets(ctx, "bob", now)
	if err != nil || len(bobs) != 1 || bobs[0].Losses() != 1 || bobs[0].Rating() != user.PetStartRating-16 {
		t.Fatalf("the hamster after the fight: %+v, %v", bobs, err)
	}
	for _, id := range []uuid.UUID{ann, bob} {
		if fights, err := pets.Fights(ctx, id); err != nil || len(fights) != 1 {
			t.Fatalf("Fights = %v, %v", fights, err)
		}
	}

	// A meal costs 5 votes, a full pet takes none; a game is free.
	if care, err = pets.Feed(ctx, ann, dragon, now); err != nil || care.Balance != 398 {
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
	if err != nil || settings.Stats.Pets != 3 || settings.Stats.Fights != 1 || settings.Stats.Spent != 655 || settings.Stats.Won != 3 {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}
	if _, err = pets.Fight(ctx, ann, dragon, "bob", now); !errors.Is(err, user.ErrPetFightsOver) {
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

	if err = pets.Release(ctx, bob, hamsterID); err != nil {
		t.Fatal(err)
	}
	if bobs, _, err = pets.UserPets(ctx, "bob", now); err != nil || len(bobs) != 0 {
		t.Fatalf("a released pet: %v, %v", bobs, err)
	}
	if err = pets.Release(ctx, bob, hamsterID); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("Release twice: %v", err)
	}
	if _, err = pets.Fight(ctx, ann, dragon, "bob", now.Add(24*time.Hour)); !errors.Is(err, user.ErrPetNotFound) {
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
