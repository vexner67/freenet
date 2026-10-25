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

func TestBusinessRepository(t *testing.T) {
	repo, pool := newRepo(t)
	businesses := postgres.NewBusinessRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Cleanup(func() {
		if err := businesses.SetRules(context.Background(), user.DefaultBusinessRules()); err != nil {
			t.Error(err)
		}
	})

	if got, err := businesses.Rules(ctx); err != nil || !reflect.DeepEqual(got, user.DefaultBusinessRules()) {
		t.Fatalf("Rules = %+v, %v; want the defaults", got, err)
	}

	def := user.DefaultBusinessRules()
	plans := def.Plans()
	plans[1].Price = 4321
	tuned, err := user.NewBusinessRules(plans, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err = businesses.SetRules(ctx, tuned); err != nil {
		t.Fatal(err)
	}
	if got, err := businesses.Rules(ctx); err != nil || !reflect.DeepEqual(got, tuned) {
		t.Fatalf("Rules = %+v, %v; want the tuned ones", got, err)
	}

	ann, err := user.New(user.Email(unique("business") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, ann); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	kiosk, _ := tuned.Plan(user.BusinessKiosk)
	cafe, _ := tuned.Plan(user.BusinessCafe)

	before, err := businesses.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err = businesses.AddBusiness(ctx, ann.ID(), user.NewBusiness(cafe, now)); err != nil {
		t.Fatal(err)
	}
	if err = businesses.AddBusiness(ctx, ann.ID(), user.NewBusiness(kiosk, now)); err != nil {
		t.Fatal(err)
	}
	if err = businesses.AddBusiness(ctx, ann.ID(), user.NewBusiness(kiosk, now)); !errors.Is(err, user.ErrBusinessOwned) {
		t.Errorf("a second kiosk: %v", err)
	}
	if err = businesses.AddBusiness(ctx, uuid.New(), user.NewBusiness(kiosk, now)); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("a business of nobody: %v", err)
	}

	owned, err := businesses.Businesses(ctx, ann.ID())
	if err != nil || len(owned) != 2 || owned[0].Kind() != user.BusinessKiosk || owned[1].Kind() != user.BusinessCafe {
		t.Fatalf("Businesses = %+v, %v; want the kiosk, then the cafe", owned, err)
	}

	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		b, err := businesses.Business(ctx, ann.ID(), user.BusinessKiosk)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(b, user.NewBusiness(kiosk, now)) {
			t.Errorf("Business = %+v; want the new kiosk", b.State())
		}

		b, _, err = b.Restocked(kiosk, 50)
		if err != nil {
			return err
		}
		b, _, err = b.Advertised(kiosk, now)
		if err != nil {
			return err
		}
		b, _, err = b.Improved(kiosk, user.PerkShelves)
		if err != nil {
			return err
		}
		if err = businesses.SaveBusiness(ctx, ann.ID(), b.At(kiosk, now.Add(2*time.Hour))); err != nil {
			return err
		}

		return businesses.Count(ctx, 15, 4)
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := businesses.Business(ctx, ann.ID(), user.BusinessKiosk)
	if err != nil || got.State().Stock >= 50 || got.State().AdUntil.IsZero() || got.Perk(user.PerkShelves) != 1 {
		t.Errorf("saved kiosk = %+v, %v", got.State(), err)
	}

	if err = businesses.DeleteBusiness(ctx, ann.ID(), user.BusinessCafe); err != nil {
		t.Fatal(err)
	}
	if _, err = businesses.Business(ctx, ann.ID(), user.BusinessCafe); !errors.Is(err, user.ErrBusinessNotFound) {
		t.Errorf("deleted cafe: %v", err)
	}
	if err = businesses.SaveBusiness(ctx, ann.ID(), user.NewBusiness(cafe, now)); !errors.Is(err, user.ErrBusinessNotFound) {
		t.Errorf("SaveBusiness of a deleted cafe: %v", err)
	}

	after, err := businesses.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Spent-before.Spent != 15 || after.Paid-before.Paid != 4 || after.Businesses < 1 {
		t.Errorf("Stats = %+v after %+v", after, before)
	}
}
