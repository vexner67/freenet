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

func TestFirmService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")
	// Late enough for the pages to be a week old.
	now := time.Now().Add(user.BonusAge + time.Hour)

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	firms := service.NewFirmService(w.users, inmem.NewFirmRepository(), w.users, inmem.TxManager{}, admins)
	farm, _ := user.DefaultFirmRules().Plan(user.FirmFarm)
	found := service.FirmAct{Action: user.FirmFound, Kind: user.FirmFarm, Name: "Колхоз"}

	if _, err = firms.Act(ctx, ann, found, now); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("found without votes: %v", err)
	}
	if view, err := firms.View(ctx, now); err != nil || len(view.City.Firms()) != 0 {
		t.Fatalf("a failed founding stays: %v", err)
	}

	for _, id := range []uuid.UUID{ann, bob} {
		if _, err = w.users.Credit(ctx, id, 2000, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
			t.Fatal(err)
		}
	}
	for name, id := range map[string]uuid.UUID{"ann": ann, "bob": bob} {
		if _, err = w.userSvc.UpdateProfile(ctx, id, profile(t, name), ""); err != nil {
			t.Fatal(err)
		}
	}

	done, err := firms.Act(ctx, ann, found, now)
	if err != nil || done.Spent != farm.Price || done.Balance != 2000-farm.Price || done.FirmID == uuid.Nil {
		t.Fatalf("found = %+v, %v", done, err)
	}
	id := done.FirmID

	if _, err = firms.Act(ctx, bob, service.FirmAct{Action: user.FirmDeposit, FirmID: id, Votes: 10}, now); !errors.Is(err, user.ErrNotDirector) {
		t.Fatalf("deposit by bob: %v", err)
	}

	// A day of work: the hours are counted by Tick.
	counted, err := firms.Tick(ctx, now.Add(24*time.Hour))
	if err != nil || counted != 24 {
		t.Fatalf("Tick = %d, %v", counted, err)
	}
	view, _ := firms.View(ctx, now)
	if f := view.City.Firms()[0]; f.Profit() <= 0 {
		t.Fatalf("the farm earned %d", f.Profit())
	}

	// Ann sells bob 40 shares, less the fee of 2 percent.
	if _, err = firms.Act(ctx, ann, service.FirmAct{Action: user.FirmOffer, FirmID: id, Shares: 40, Votes: 10}, now); err != nil {
		t.Fatal(err)
	}
	view, _ = firms.View(ctx, now)
	offer := view.City.Offers()[0].ID
	if done, err = firms.Act(ctx, bob, service.FirmAct{Action: user.FirmBuyShares, OfferID: offer, Shares: 40}, now); err != nil || done.Spent != 400 {
		t.Fatalf("buy shares = %+v, %v", done, err)
	}
	if votes := w.votes(t, ann); votes != 2000-farm.Price+392 {
		t.Fatalf("ann has %d votes", votes)
	}

	// Dividends of 100 votes: 60 to ann, 40 to bob.
	done, err = firms.Act(ctx, ann, service.FirmAct{Action: user.FirmDividend, FirmID: id, Votes: 100}, now)
	if err != nil || done.Got != 60 {
		t.Fatalf("dividend = %+v, %v", done, err)
	}
	if votes := w.votes(t, bob); votes != 2000-400+40 {
		t.Fatalf("bob has %d votes", votes)
	}

	if _, err = firms.Settings(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Settings by ann: %v", err)
	}
	settings, err := firms.Settings(ctx, admin)
	if err != nil || settings.Firms != 1 || settings.Stats.Spent != farm.Price || settings.Stats.Paid != 100 {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}

	held, err := firms.UserFirms(ctx, "bob", now)
	if err != nil || len(held) != 1 || held[0].Shares != 40 || held[0].Director {
		t.Fatalf("UserFirms of bob = %+v, %v", held, err)
	}
}

func TestFirmPayoutsWaitForAWeek(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann := w.person(t, "ann")
	firms := service.NewFirmService(w.users, inmem.NewFirmRepository(), w.users, inmem.TxManager{}, user.Admins{})

	_, err := firms.Act(ctx, ann, service.FirmAct{Action: user.FirmDividend, FirmID: uuid.New(), Votes: 1}, time.Now())
	if !errors.Is(err, user.ErrFirmNotYet) {
		t.Fatalf("dividends of a new page: %v", err)
	}
}
