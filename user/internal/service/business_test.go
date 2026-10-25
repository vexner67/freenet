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

func TestBusinessService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	// Late enough for the page to be a week old.
	now := time.Now().Add(user.BonusAge + time.Hour)

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	businesses := service.NewBusinessService(w.users, inmem.NewBusinessRepository(), w.users, inmem.TxManager{}, admins)
	kiosk, _ := user.DefaultBusinessRules().Plan(user.BusinessKiosk)

	if _, err = businesses.Buy(ctx, ann, user.BusinessKiosk, now); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Buy without votes: %v", err)
	}
	if view, err := businesses.Businesses(ctx, ann, now); err != nil || len(view.Businesses) != 0 {
		t.Fatalf("Businesses after a failed buy = %+v, %v", view, err)
	}

	if _, err = w.users.Credit(ctx, ann, 1000, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}

	bought, err := businesses.Buy(ctx, ann, user.BusinessKiosk, now)
	if err != nil || bought.Spent != kiosk.Price || bought.Balance != 1000-kiosk.Price {
		t.Fatalf("Buy = %+v, %v", bought, err)
	}
	if _, err = businesses.Buy(ctx, ann, user.BusinessKiosk, now); !errors.Is(err, user.ErrBusinessOwned) {
		t.Fatalf("a second kiosk: %v", err)
	}
	if _, err = businesses.Act(ctx, ann, user.BusinessCafe, service.BusinessAct{Action: user.BusinessRestock, Amount: 1}, now); !errors.Is(err, user.ErrBusinessNotFound) {
		t.Fatalf("Act on a cafe not owned: %v", err)
	}

	stocked, err := businesses.Act(ctx, ann, user.BusinessKiosk, service.BusinessAct{Action: user.BusinessRestock, Amount: 300}, now)
	if err != nil || stocked.Spent != 30 || stocked.Balance != 1000-kiosk.Price-30 {
		t.Fatalf("restock = %+v, %v", stocked, err)
	}

	improved, err := businesses.Act(ctx, ann, user.BusinessKiosk, service.BusinessAct{Action: user.BusinessImprove, Perk: user.PerkDecor}, now)
	if err != nil || improved.Spent != kiosk.PerkPrice(1) || improved.Business.Perk(user.PerkDecor) != 1 {
		t.Fatalf("improve = %+v, %v", improved, err)
	}
	spent := kiosk.Price + 30 + improved.Spent

	// Taking votes out is for pages a week old with a filled profile.
	later := now.Add(10 * time.Hour)
	if _, err = businesses.Act(ctx, ann, user.BusinessKiosk, service.BusinessAct{Action: user.BusinessCollect}, later); !errors.Is(err, user.ErrBusinessNotYet) {
		t.Fatalf("collect of a new page: %v", err)
	}
	if _, err = w.userSvc.UpdateProfile(ctx, ann, profile(t, "ann"), ""); err != nil {
		t.Fatal(err)
	}

	collected, err := businesses.Act(ctx, ann, user.BusinessKiosk, service.BusinessAct{Action: user.BusinessCollect}, later)
	if err != nil || collected.Got < 1 || collected.Tax < 1 || collected.Balance != 1000-spent+collected.Got {
		t.Fatalf("collect = %+v, %v", collected, err)
	}

	sold, err := businesses.Act(ctx, ann, user.BusinessKiosk, service.BusinessAct{Action: user.BusinessSell}, later)
	if err != nil || !sold.Closed || sold.Got != (kiosk.Price+improved.Spent)/2 {
		t.Fatalf("sell = %+v, %v", sold, err)
	}
	if view, err := businesses.Businesses(ctx, ann, later); err != nil || len(view.Businesses) != 0 {
		t.Fatalf("Businesses after the sale = %+v, %v", view, err)
	}

	// Only the administrators read and tune the businesses.
	def := user.DefaultBusinessRules()
	tuned, err := user.NewBusinessRules(def.Plans(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if err = businesses.SetRules(ctx, ann, tuned); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetRules by ann: %v", err)
	}
	if err = businesses.SetRules(ctx, admin, tuned); err != nil {
		t.Fatal(err)
	}

	settings, err := businesses.Settings(ctx, admin)
	if err != nil || settings.Rules.Tax() != 10 ||
		settings.Stats != (user.BusinessStats{Spent: spent, Paid: collected.Got + sold.Got}) {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}
}
