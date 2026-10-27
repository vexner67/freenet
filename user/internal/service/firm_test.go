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
	// The hour going on is shown as it would end; an hour not yet counted
	// has no forecast.
	if view, _ = firms.View(ctx, now.Add(24*time.Hour)); view.Forecast[id].Hour.Made == 0 {
		t.Fatalf("forecast = %+v", view.Forecast)
	}
	if view, _ = firms.View(ctx, now.Add(26*time.Hour)); view.Forecast != nil {
		t.Fatalf("forecast of hours not counted = %+v", view.Forecast)
	}

	// Bob puts 500 votes on his broker account and buys 10 new shares of
	// the farm at the price of the exchange.
	fund := service.FirmAct{Action: user.FirmFund, Votes: 500}
	if done, err = firms.Act(ctx, bob, fund, now); err != nil || done.Spent != 500 || done.Balance != 1500 {
		t.Fatalf("fund = %+v, %v", done, err)
	}
	buy := service.FirmAct{Action: user.FirmStockOrder, Stock: user.StockOrder{FirmID: id, Buy: true, Shares: 10}}
	if done, err = firms.Act(ctx, bob, buy, now); err != nil || done.Fill.Shares != 10 || done.Fill.Money <= 0 {
		t.Fatalf("buy shares = %+v, %v", done, err)
	}
	paid := done.Fill.Money

	// Dividends of 110 votes: 100 to ann, 10 to bob.
	done, err = firms.Act(ctx, ann, service.FirmAct{Action: user.FirmDividend, FirmID: id, Votes: 110}, now)
	if err != nil || done.Got != 100 {
		t.Fatalf("dividend = %+v, %v", done, err)
	}
	if votes := w.votes(t, bob); votes != 2000-500+10 {
		t.Fatalf("bob has %d votes", votes)
	}

	// Bob takes the rest of his account out.
	left := (500*100 - paid) / 100
	withdraw := service.FirmAct{Action: user.FirmWithdraw, Votes: left}
	if done, err = firms.Act(ctx, bob, withdraw, now); err != nil || done.Got != left {
		t.Fatalf("withdraw = %+v, %v", done, err)
	}
	if _, err = firms.Act(ctx, bob, withdraw, now); !errors.Is(err, user.ErrAccount) {
		t.Fatalf("withdraw twice: %v", err)
	}

	if _, err = firms.Settings(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Settings by ann: %v", err)
	}
	settings, err := firms.Settings(ctx, admin)
	if err != nil || settings.Firms != 1 || settings.Stats.Spent != farm.Price+500 || settings.Stats.Paid != 110+left {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}

	held, err := firms.UserFirms(ctx, "bob", now)
	if err != nil || len(held) != 1 || held[0].Shares != 10 || held[0].Director {
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

// TestFirmTakeover: bob offers to buy ann's shares, ann sells him most of
// them and he comes to direct the farm.
func TestFirmTakeover(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann, bob := w.person(t, "ann"), w.person(t, "bob")
	now := time.Now()
	firms := service.NewFirmService(w.users, inmem.NewFirmRepository(), w.users, inmem.TxManager{}, user.Admins{})

	for _, id := range []uuid.UUID{ann, bob} {
		if _, err := w.users.Credit(ctx, id, 2000, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
			t.Fatal(err)
		}
	}
	done, err := firms.Act(ctx, ann, service.FirmAct{Action: user.FirmFound, Kind: user.FirmFarm, Name: "Колхоз"}, now)
	if err != nil {
		t.Fatal(err)
	}
	id := done.FirmID
	if _, err = firms.Act(ctx, bob, service.FirmAct{Action: user.FirmFund, Votes: 1000}, now); err != nil {
		t.Fatal(err)
	}

	f, _ := done.View.City.Firm(id)
	offer := service.FirmAct{Action: user.FirmTender, Stock: user.StockOrder{FirmID: id, Shares: 60, Price: f.Price() + 100}}
	if done, err = firms.Act(ctx, bob, offer, now); err != nil || len(done.View.City.Tenders()) != 1 {
		t.Fatalf("tender = %v", err)
	}
	tenderID := done.View.City.Tenders()[0].ID

	sell := service.FirmAct{Action: user.FirmTenderSell, OrderID: tenderID, Stock: user.StockOrder{Shares: 60}}
	if done, err = firms.Act(ctx, ann, sell, now); err != nil || done.Fill.Shares != 60 {
		t.Fatalf("selling to the offer = %+v, %v", done.Fill, err)
	}
	if f, _ = done.View.City.Firm(id); f.DirectorID() != bob {
		t.Errorf("bob holds 60 of 100 and does not direct")
	}

	closing := service.FirmAct{Action: user.FirmTenderClose, OrderID: tenderID}
	if _, err = firms.Act(ctx, bob, closing, now); !errors.Is(err, user.ErrTenderNotFound) {
		t.Errorf("closing an offer done: %v", err)
	}
}
