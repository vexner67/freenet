package postgres_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestGuestRepository(t *testing.T) {
	repo, pool := newRepo(t)
	guests := postgres.NewGuestRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ann, bob, eve := savedUser(t, repo, "guest"), savedUser(t, repo, "guest"), savedUser(t, repo, "guest")
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := guests.Visit(ctx, ann, uuid.New(), now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Visit of nobody: %v", err)
	}
	for i, id := range []uuid.UUID{bob, eve} {
		if err := guests.Visit(ctx, ann, id, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// An older visit of Bob does not take his last one back.
	if err := guests.Visit(ctx, ann, bob, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := guests.Visit(ctx, ann, bob, now); err != nil {
		t.Fatal(err)
	}

	page, err := guests.Guests(ctx, ann, now.Add(-time.Hour), nil, 1)
	if err != nil || len(page) != 1 || page[0].GuestID != bob || !page[0].VisitedAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("Guests = %+v, %v", page, err)
	}
	next, err := guests.Guests(ctx, ann, now.Add(-time.Hour), &service.GuestKey{VisitedAt: page[0].VisitedAt, GuestID: bob}, 10)
	if err != nil || len(next) != 1 || next[0].GuestID != eve {
		t.Fatalf("the next page = %+v, %v", next, err)
	}

	if n, err := guests.CountSince(ctx, ann, now.Add(30*time.Second), 10); err != nil || n != 2 {
		t.Fatalf("CountSince = %d, %v", n, err)
	}
	if n, _ := guests.CountSince(ctx, ann, now.Add(30*time.Second), 1); n != 1 {
		t.Fatalf("CountSince over the limit = %d", n)
	}

	if at, err := guests.SeenAt(ctx, ann); err != nil || !at.IsZero() {
		t.Fatalf("SeenAt of nobody = %v, %v", at, err)
	}
	if err = guests.MarkSeen(ctx, ann, now); err != nil {
		t.Fatal(err)
	}
	if err = guests.MarkSeen(ctx, ann, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if at, err := guests.SeenAt(ctx, ann); err != nil || !at.Equal(now) {
		t.Fatalf("SeenAt = %v, %v", at, err)
	}
}

func TestPetRepository(t *testing.T) {
	repo, pool := newRepo(t)
	pets := postgres.NewPetRepository(pool)
	wallets := postgres.NewWalletRepository(pool)
	tops := postgres.NewTopRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Cleanup(func() {
		if err := pets.SetShop(context.Background(), user.DefaultPetShop()); err != nil {
			t.Error(err)
		}
	})

	shop, err := user.NewPetShop(user.DefaultPetShop().Prices(), 7, 2, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err = pets.SetShop(ctx, shop); err != nil {
		t.Fatal(err)
	}
	if got, err := pets.Shop(ctx); err != nil || !reflect.DeepEqual(got, shop) {
		t.Fatalf("Shop = %+v, %v", got, err)
	}

	ann, bob := savedUser(t, repo, "pet"), savedUser(t, repo, "pet")
	now := time.Now().UTC().Truncate(time.Microsecond)

	if _, err = pets.Pet(ctx, ann, uuid.New()); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("Pet of nobody: %v", err)
	}

	// A rating nobody else has keeps the test apart from other pets.
	rating := 100_000 + rand.IntN(1_000_000)
	dragon := user.RestorePet(user.PetState{
		ID: uuid.Must(uuid.NewV7()), Kind: user.PetDragon, Name: "Смауг", BornAt: now, Satiety: 90, Joy: 90, CountedAt: now,
		Experience: 500, Rating: rating,
	})
	if err = pets.AddPet(ctx, ann, dragon); err != nil {
		t.Fatal(err)
	}
	// Ann keeps up to ten pets.
	for range user.MaxPets - 1 {
		cat := user.RestorePet(user.PetState{
			ID: uuid.Must(uuid.NewV7()), Kind: user.PetCat, Name: "Кот", BornAt: now, CountedAt: now, Rating: user.PetStartRating,
		})
		if err = pets.AddPet(ctx, ann, cat); err != nil {
			t.Fatal(err)
		}
	}
	extra := user.NewPet(user.PetCat, "Кот", now)
	if err = pets.AddPet(ctx, ann, extra); !errors.Is(err, user.ErrTooManyPets) {
		t.Fatalf("an eleventh pet: %v", err)
	}
	if err = pets.AddPet(ctx, uuid.New(), extra); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("a pet of nobody: %v", err)
	}
	if got, err := pets.Pet(ctx, ann, dragon.ID()); err != nil || !reflect.DeepEqual(got.State(), dragon.State()) {
		t.Fatalf("Pet = %+v, %v", got, err)
	}
	if all, err := pets.Pets(ctx, ann); err != nil || len(all) != user.MaxPets || all[0].ID() != dragon.ID() {
		t.Fatalf("Pets = %v, %v", all, err)
	}

	hamster := user.RestorePet(user.PetState{
		ID: uuid.Must(uuid.NewV7()), Kind: user.PetHamster, Name: "Хома", BornAt: now, Satiety: 80, Joy: 80, CountedAt: now, Rating: rating + 5,
	})
	if err = pets.AddPet(ctx, bob, hamster); err != nil {
		t.Fatal(err)
	}

	if _, err = pets.Pet(ctx, ann, hamster.ID()); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("Pet of another: %v", err)
	}
	if owner, pet, err := pets.Opponent(ctx, ann, rating, now); err != nil || owner != bob || pet != hamster.ID() {
		t.Fatalf("Opponent = %v, %v, %v", owner, pet, err)
	}

	fought, defended, bout, err := dragon.Fought(hamster, shop, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = pets.SavePet(ctx, fought); err != nil {
		t.Fatal(err)
	}
	if err = pets.SavePet(ctx, defended); err != nil {
		t.Fatal(err)
	}
	if got, err := pets.LockPet(ctx, ann, dragon.ID()); err != nil || !reflect.DeepEqual(got.State(), fought.State()) {
		t.Fatalf("LockPet after the fight = %+v, %v; want %+v", got.State(), err, fought.State())
	}

	fight := user.PetFight{ID: uuid.Must(uuid.NewV7()), AttackerID: ann, DefenderID: bob, Bout: bout, At: now}
	if err = pets.AddFight(ctx, fight); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{ann, bob} {
		if got, err := pets.Fights(ctx, id, 10); err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], fight) {
			t.Fatalf("Fights = %+v, %v", got, err)
		}
	}

	before, err := pets.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = pets.Count(ctx, 50, 3); err != nil {
		t.Fatal(err)
	}
	if after, _ := pets.Stats(ctx); after.Spent != before.Spent+50 || after.Won != before.Won+3 || after.Pets < 2 || after.Fights < 1 {
		t.Fatalf("Stats = %+v after %+v", after, before)
	}

	// The boards: the pet of Ann, her votes and what she earned.
	board, err := tops.Pets(ctx, now, user.MaxTop)
	if err != nil || !hasTop(board, ann, int64(fought.Rating())) || board[0].PetKind == "" {
		t.Fatalf("Pets = %+v, %v", board, err)
	}
	if _, err = wallets.Credit(ctx, ann, 40, 0, user.Entry(user.VoteMine, uuid.Nil, "stone")); err != nil {
		t.Fatal(err)
	}
	if _, err = wallets.Credit(ctx, ann, 2, 0, user.Entry(user.VotePet, uuid.Nil, "fight")); err != nil {
		t.Fatal(err)
	}
	miners, err := tops.Earned(ctx, user.TopMiners.Kinds(), now.Add(-time.Minute), now, 1_000_000)
	if err != nil || !hasTop(miners, ann, 40) {
		t.Fatalf("Earned = %+v, %v", miners, err)
	}
	rich, err := tops.Rich(ctx, now, 1_000_000)
	if err != nil || !hasTop(rich, ann, 42) {
		t.Fatalf("Rich = %+v, %v", rich, err)
	}

	if err = pets.DeletePet(ctx, ann, hamster.ID()); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("DeletePet of the pet of another: %v", err)
	}
	if err = pets.DeletePet(ctx, bob, hamster.ID()); err != nil {
		t.Fatal(err)
	}
	if err = pets.DeletePet(ctx, bob, hamster.ID()); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("DeletePet twice: %v", err)
	}
	if err = pets.SavePet(ctx, defended); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("SavePet of no pet: %v", err)
	}
}

func hasTop(entries []user.TopEntry, id uuid.UUID, value int64) bool {
	for _, e := range entries {
		if e.UserID == id {
			return e.Value == value
		}
	}

	return false
}

// savedUser is a new saved user.
func savedUser(t *testing.T, repo *postgres.UserRepository, prefix string) uuid.UUID {
	t.Helper()

	u, err := user.New(user.Email(unique(prefix) + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(context.Background(), u); err != nil {
		t.Fatal(err)
	}

	return u.ID()
}
