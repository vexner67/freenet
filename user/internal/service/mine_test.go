package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestMineService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	now := time.Now()

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// Every roll is the next of these; past them — the last, nothing.
	rolls := []int64{}
	mines := service.NewMineService(w.users, inmem.NewMineRepository(), w.users, inmem.TxManager{}, admins).
		WithRoll(func(n int64) (int64, error) {
			if len(rolls) == 0 {
				return n - 1, nil
			}
			roll := rolls[0]
			rolls = rolls[1:]

			return roll, nil
		})

	mine := user.DefaultMine()
	stone, _ := mine.Vein(user.OreStone)
	events := mine.Events()

	got, err := mines.Mine(ctx, ann, now)
	if err != nil || got.Miner.Energy() != mine.Energy() || got.NextEnergy != 0 {
		t.Fatalf("Mine = %+v, %v", got, err)
	}

	if _, err = mines.Dig(ctx, ann, user.OreGold, now); !errors.Is(err, user.ErrOreLocked) {
		t.Fatalf("Dig of gold: %v", err)
	}

	// A plain dig, then a chest, which is paid at once.
	rolls = []int64{user.MineOdds - 1, 0, 0}
	dug, err := mines.Dig(ctx, ann, user.OreStone, now)
	if err != nil || dug.Haul.Units != stone.Least || dug.Haul.Votes != 0 || dug.View.Miner.Energy() != mine.Energy()-1 ||
		dug.View.NextEnergy != mine.Regen() {
		t.Fatalf("Dig = %+v, %v", dug, err)
	}

	rolls = []int64{int64(events.Nugget), 1, 2}
	if dug, err = mines.Dig(ctx, ann, user.OreStone, now); err != nil || dug.Haul.Event != user.MineEventChest ||
		dug.Haul.Votes != events.ChestLeast+2 || dug.Balance != events.ChestLeast+2 {
		t.Fatalf("Dig with a chest = %+v, %v", dug, err)
	}

	// Selling is for pages a week old with a filled profile.
	if _, err = mines.Sell(ctx, ann, user.OreStone, now); !errors.Is(err, user.ErrMineNotYet) {
		t.Fatalf("Sell of a new page: %v", err)
	}
	if _, err = w.userSvc.UpdateProfile(ctx, ann, profile(t, "ann"), ""); err != nil {
		t.Fatal(err)
	}

	later := now.Add(user.BonusAge + time.Hour)
	if _, err = mines.Sell(ctx, ann, user.OreCoal, later); !errors.Is(err, user.ErrNothingToSell) {
		t.Fatalf("Sell of no coal: %v", err)
	}

	units := 2*stone.Least + 1
	sold, err := mines.Sell(ctx, ann, "", later)
	if err != nil || sold.Units != units || sold.Votes != stone.SaleValue(units) ||
		sold.Balance != events.ChestLeast+2+sold.Votes || len(sold.View.Miner.Backpack()) != 0 ||
		sold.View.Miner.Energy() != mine.Energy() {
		t.Fatalf("Sell of all = %+v, %v", sold, err)
	}
	if _, err = mines.Sell(ctx, ann, "", later); !errors.Is(err, user.ErrNothingToSell) {
		t.Fatalf("Sell of an empty backpack: %v", err)
	}

	// Only the administrators read and tune the mine.
	tuned, err := user.NewMine(mine.Veins(), 50, time.Minute, events)
	if err != nil {
		t.Fatal(err)
	}
	if err = mines.SetMine(ctx, ann, tuned); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetMine by ann: %v", err)
	}
	if _, err = mines.Settings(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Settings by ann: %v", err)
	}
	if err = mines.SetMine(ctx, admin, tuned); err != nil {
		t.Fatal(err)
	}

	settings, err := mines.Settings(ctx, admin)
	if err != nil || settings.Mine.Energy() != 50 ||
		settings.Stats != (user.MineStats{Digs: 2, Sold: sold.Votes, Chests: events.ChestLeast + 2, Miners: 1}) {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}
}
