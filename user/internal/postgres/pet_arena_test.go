package postgres_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPetArenaRepository(t *testing.T) {
	repo, pool := newRepo(t)
	pets := postgres.NewPetRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ann, bob := savedUser(t, repo, "arena"), savedUser(t, repo, "arena")
	now := time.Now().UTC().Truncate(time.Microsecond)

	// The boss and the tournament of the week are made by the first lock
	// and kept by it after.
	boss := user.NewPetBoss(now)
	locked, err := pets.LockBoss(ctx, boss)
	if err != nil || !locked.Week().Equal(boss.Week()) {
		t.Fatalf("LockBoss = %+v, %v", locked, err)
	}
	if err = pets.SaveBoss(ctx, locked); err != nil {
		t.Fatal(err)
	}
	if got, err := pets.Boss(ctx, boss.Week()); err != nil || got.Health() != locked.Health() {
		t.Fatalf("Boss = %+v, %v", got, err)
	}
	if _, err = pets.Boss(ctx, boss.Week().AddDate(-100, 0, 0)); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Boss of a week without one: %v", err)
	}

	tournament := user.NewPetTournament(now)
	lockedTournament, err := pets.LockTournament(ctx, tournament)
	if err != nil || !lockedTournament.Week().Equal(tournament.Week()) {
		t.Fatalf("LockTournament = %+v, %v", lockedTournament, err)
	}
	if err = pets.SaveTournament(ctx, lockedTournament); err != nil {
		t.Fatal(err)
	}
	if got, err := pets.Tournament(ctx, tournament.Week()); err != nil || !got.Week().Equal(tournament.Week()) {
		t.Fatalf("Tournament = %+v, %v", got, err)
	}

	// A rating above all others puts the pet first among the best.
	rating := user.PetStartRating
	if top, err := pets.Best(ctx, now, 1); err != nil {
		t.Fatal(err)
	} else if len(top) == 1 {
		rating = top[0].Pet.Rating() + 1
	}
	best := user.RestorePet(user.PetState{
		ID: uuid.Must(uuid.NewV7()), Kind: user.PetDragon, Name: "Смауг", BornAt: now, CountedAt: now,
		Rating: rating, Stance: user.PetAttack, SalePrice: 1,
	})
	if err = pets.AddPet(ctx, ann, best); err != nil {
		t.Fatal(err)
	}
	top, err := pets.Best(ctx, now, 1)
	if err != nil || len(top) != 1 || top[0].Owner != ann || top[0].Pet.ID() != best.ID() {
		t.Fatalf("Best = %+v, %v", top, err)
	}

	listed, err := pets.Listings(ctx, now, 1000)
	if err != nil || !slices.ContainsFunc(listed, func(c user.PetContender) bool { return c.Pet.ID() == best.ID() && c.Owner == ann }) {
		t.Fatalf("Listings = %+v, %v", listed, err)
	}
	if _, err = pets.LockListed(ctx, uuid.New()); !errors.Is(err, user.ErrPetNotFound) {
		t.Fatalf("LockListed of nothing: %v", err)
	}
	if err = pets.MovePet(ctx, best.ID(), bob); err != nil {
		t.Fatal(err)
	}
	if got, err := pets.LockListed(ctx, best.ID()); err != nil || got.Owner != bob {
		t.Fatalf("LockListed after MovePet = %+v, %v", got, err)
	}
	if err = pets.MovePet(ctx, best.ID(), uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("MovePet to nobody: %v", err)
	}

	quests, err := pets.LockQuests(ctx, ann)
	if err != nil {
		t.Fatal(err)
	}
	quests, _ = quests.Did(user.PetQuestFight, 1, now)
	if err = pets.SaveQuests(ctx, ann, quests); err != nil {
		t.Fatal(err)
	}
	if got, err := pets.LockQuests(ctx, ann); err != nil || got.ProgressAt(user.PetQuestFight, now) != quests.ProgressAt(user.PetQuestFight, now) {
		t.Fatalf("LockQuests = %+v, %v", got, err)
	}
	if _, err = pets.LockQuests(ctx, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("LockQuests of nobody: %v", err)
	}

	if err = pets.Achieve(ctx, ann, []user.PetAchievement{user.PetFirstWin, user.PetBossSlayer}, now); err != nil {
		t.Fatal(err)
	}
	if err = pets.Achieve(ctx, ann, []user.PetAchievement{user.PetFirstWin}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := pets.Achievements(ctx, ann)
	if err != nil || len(got) != 2 || !got[0].At.Equal(now) {
		t.Fatalf("Achievements = %+v, %v", got, err)
	}

	// Closing a season carries every rating, so it runs in a transaction
	// rolled back.
	errRollback := errors.New("rollback")
	err = pgpool.NewTxManager(pool).WithinTx(ctx, func(ctx context.Context) error {
		season := user.PetSeasonResult{
			Season: time.Date(3000+rand.IntN(1000), 1, 1, 0, 0, 0, 0, time.UTC),
			Places: []user.PetSeasonPlace{{Owner: ann, PetID: best.ID(), Kind: user.PetDragon, Name: "Смауг", Rating: best.Rating(), Prize: 100}},
		}
		if closed, err := pets.CloseSeason(ctx, season); err != nil || !closed {
			t.Fatalf("CloseSeason = %v, %v", closed, err)
		}
		if closed, err := pets.CloseSeason(ctx, season); err != nil || closed {
			t.Fatalf("CloseSeason twice = %v, %v", closed, err)
		}
		seasons, err := pets.Seasons(ctx, 1)
		if err != nil || len(seasons) != 1 || !seasons[0].Season.Equal(season.Season) || len(seasons[0].Places) != 1 {
			t.Fatalf("Seasons = %+v, %v", seasons, err)
		}

		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
}
