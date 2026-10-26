package postgres_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestFirmRepository(t *testing.T) {
	repo, pool := newRepo(t)
	firms := postgres.NewFirmRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Cleanup(func() {
		if err := firms.SetRules(context.Background(), user.DefaultFirmRules()); err != nil {
			t.Error(err)
		}
	})

	if got, err := firms.Rules(ctx); err != nil || !reflect.DeepEqual(got, user.DefaultFirmRules()) {
		t.Fatalf("Rules = %+v, %v; want the defaults", got, err)
	}

	tuned := user.DefaultFirmRules()
	tuned.Plans[1].Price = 4321
	tuned.Goods[2].Norm = 77
	tuned.Fee = 5
	if err := firms.SetRules(ctx, tuned); err != nil {
		t.Fatal(err)
	}
	if got, err := firms.Rules(ctx); err != nil || !reflect.DeepEqual(got, tuned) {
		t.Fatalf("Rules = %+v, %v; want the tuned ones", got, err)
	}

	ann, err := user.New(user.Email(unique("firm") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, ann); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Hour)
	city := user.NewCity(now)
	mill, err := city.Found(tuned, ann.ID(), user.FirmMill, "Мельница", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = city.BuyFromState(tuned, ann.ID(), mill.ID(), user.GoodGrain, 100, now); err != nil {
		t.Fatal(err)
	}
	if _, err = city.Place(tuned, ann.ID(), mill.ID(), user.ExchangeOrder{Good: user.GoodGrain, Buy: true, Price: 10, Units: 5}, now); err != nil {
		t.Fatal(err)
	}
	if err = city.Fund(ann.ID(), 500); err != nil {
		t.Fatal(err)
	}
	order := user.StockOrder{FirmID: mill.ID(), Buy: true, Price: 1, Shares: 3}
	if _, err = city.PlaceStock(tuned, ann.ID(), order, now); err != nil {
		t.Fatal(err)
	}
	if err = city.Issue(ann.ID(), mill.ID(), 10); err != nil {
		t.Fatal(err)
	}
	city.Tick(tuned, now.Add(2*time.Hour))

	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := firms.City(ctx, true); err != nil {
			return err
		}

		return firms.SaveCity(ctx, city)
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := firms.City(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.State(), city.State()) {
		t.Fatalf("City = %+v\nwant %+v", got.State(), city.State())
	}

	if err = firms.SaveCity(ctx, user.NewCity(now)); err != nil {
		t.Fatal(err)
	}
}
