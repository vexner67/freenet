package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestMineRepository(t *testing.T) {
	repo, pool := newRepo(t)
	mines := postgres.NewMineRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts it as the default mine; the test leaves it so.
	t.Cleanup(func() {
		if err := mines.SetMine(context.Background(), user.DefaultMine()); err != nil {
			t.Error(err)
		}
	})

	if got, err := mines.Mine(ctx); err != nil || !reflect.DeepEqual(got, user.DefaultMine()) {
		t.Fatalf("Mine = %+v, %v; want the default", got, err)
	}

	def := user.DefaultMine()
	events := def.Events()
	events.Chest = 1234
	tuned, err := user.NewMine(def.Veins(), 20, time.Minute, events)
	if err != nil {
		t.Fatal(err)
	}
	if err = mines.SetMine(ctx, tuned); err != nil {
		t.Fatal(err)
	}
	if got, err := mines.Mine(ctx); err != nil || !reflect.DeepEqual(got, tuned) {
		t.Fatalf("Mine = %+v, %v; want the tuned one", got, err)
	}

	ann, err := user.New(user.Email(unique("mine") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, ann); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	fresh := user.NewMiner(tuned, now)

	before, err := mines.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// A miner comes fresh, is kept and stays locked in a transaction.
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		m, err := mines.Miner(ctx, ann.ID(), fresh)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(m, fresh) {
			t.Errorf("Miner of a new page = %+v; want %+v", m, fresh)
		}

		dug, _, err := m.Dig(tuned, user.OreStone, now, user.MineRolls{Event: user.MineOdds - 1, Yield: 3})
		if err != nil {
			return err
		}
		if err = mines.SaveMiner(ctx, ann.ID(), dug); err != nil {
			return err
		}
		if err = mines.CountDig(ctx, 7); err != nil {
			return err
		}

		return mines.CountSale(ctx, 5)
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := mines.Miner(ctx, ann.ID(), fresh)
	if err != nil {
		t.Fatal(err)
	}
	stone, _ := tuned.Vein(user.OreStone)
	if got.Digs() != 1 || got.Experience() != stone.Gain || got.Backpack()[user.OreStone] != stone.Least+3 ||
		got.Energy() != tuned.Energy()-1 || !got.CountedAt().Equal(now) {
		t.Fatalf("Miner after a dig = %+v", got)
	}

	after, err := mines.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Digs != before.Digs+1 || after.Chests != before.Chests+7 || after.Sold != before.Sold+5 || after.Miners < 1 {
		t.Fatalf("Stats = %+v; before %+v", after, before)
	}

	if _, err = mines.Miner(ctx, uuid.New(), fresh); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Miner of nobody: %v", err)
	}
	if err = mines.SaveMiner(ctx, uuid.New(), got); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("SaveMiner of nobody: %v", err)
	}
}
