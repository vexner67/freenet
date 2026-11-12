package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

type closingMemory struct {
	assets    user.ClosingAssets
	report    service.ClosingReport
	frozen    bool
	paid      map[uuid.UUID]bool
	interrupt uuid.UUID
}

func (m *closingMemory) Assets(context.Context, time.Time) (user.ClosingAssets, error) {
	return m.assets, nil
}

func (m *closingMemory) Existing(_ context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	return ids, nil
}

func (m *closingMemory) Lock(context.Context) error { return nil }

func (m *closingMemory) Plan(context.Context) (service.ClosingReport, bool, error) {
	return m.report, m.frozen, nil
}

func (m *closingMemory) SavePlan(_ context.Context, _ time.Time, report service.ClosingReport) error {
	m.report, m.frozen = report, true
	return nil
}

func (m *closingMemory) Paid(_ context.Context, id uuid.UUID) (bool, error) {
	if id == m.interrupt {
		return false, context.Canceled
	}

	return m.paid[id], nil
}

func (m *closingMemory) MarkPaid(_ context.Context, id uuid.UUID) error {
	m.paid[id] = true
	return nil
}

func TestGamesClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	wallets := inmem.NewUserRepository()
	newUser := func(email user.Email) uuid.UUID {
		t.Helper()
		person, err := user.New(email)
		if err != nil {
			t.Fatal(err)
		}
		if err = wallets.Save(ctx, person); err != nil {
			t.Fatal(err)
		}
		return person.ID()
	}
	ann, bob := newUser("ann@example.test"), newUser("bob@example.test")
	memory := &closingMemory{
		assets: closingFixture(now, ann, bob),
		paid:   make(map[uuid.UUID]bool),
	}
	games := service.NewGamesService(memory, memory, wallets, inmem.TxManager{})
	preview, err := games.Close(ctx, false, clock)
	if err != nil || memory.frozen || len(memory.paid) != 0 {
		t.Fatalf("preview = %+v, %v; frozen %v, paid %v", preview, err, memory.frozen, memory.paid)
	}
	want := map[string]int64{"broker": 5, "firm": 1488, "table": 35, "stake": 33, "ore": 3, "pet": 3100, "clothes": 40}
	if !reflect.DeepEqual(preview.ByReason, want) || preview.Total != 4704 || len(preview.Accounts) != 2 {
		t.Fatalf("preview = %+v, want %+v", preview, want)
	}

	memory.interrupt = bob
	if _, err = games.Close(ctx, true, clock); !errors.Is(err, context.Canceled) || !memory.paid[ann] || memory.paid[bob] {
		t.Fatalf("interrupted close = %v, paid %v", err, memory.paid)
	}
	memory.interrupt = uuid.Nil
	now = now.Add(time.Hour)
	for range 2 {
		report, err := games.Close(ctx, true, clock)
		if err != nil || !reflect.DeepEqual(report, preview) {
			t.Fatalf("close = %+v, %v; want frozen preview %+v", report, err, preview)
		}
	}
	for _, a := range preview.Accounts {
		person, err := wallets.GetByID(ctx, a.UserID)
		if err != nil {
			t.Fatal(err)
		}
		if got := person.Wallet().Votes; got != a.Total {
			t.Fatalf("wallet = %d, want %d", got, a.Total)
		}
	}
}

func closingFixture(at time.Time, ann, bob uuid.UUID) user.ClosingAssets {
	firm := uuid.New()
	return user.ClosingAssets{
		Rules: user.DefaultFirmRules(),
		City: user.RestoreCity(user.CityState{
			Firms: []user.FirmState{{ID: firm, Kind: user.FirmFarm, DirectorID: ann, Level: 2, Improved: 100, Cash: 2500}},
			Holdings: []user.Holding{{FirmID: firm, UserID: ann, Shares: 60}, {FirmID: firm, UserID: bob, Shares: 15},
				{FirmID: firm, UserID: user.MarketID, Shares: 25}},
			Orders:   []user.ExchangeOrder{{FirmID: firm, Buy: true, Price: 30, Units: 200}},
			Accounts: []user.Account{{UserID: ann, Cash: 155}},
			StockOrders: []user.StockOrder{{FirmID: firm, UserID: ann, Buy: true, Price: 25, Shares: 7},
				{FirmID: firm, UserID: ann, Price: 30, Shares: 5}},
			Tenders: []user.Tender{{FirmID: firm, UserID: ann, Price: 20, Shares: 10}},
		}),
		Tables: []*user.HoldemTable{
			user.RestoreHoldemTable(uuid.New(), user.HoldemState{Street: user.StreetFlop, Players: []user.TableSeat{{UserID: ann, Stack: 12, Put: 5}}}),
			user.RestoreHoldemTable(uuid.New(), user.HoldemState{Street: user.StreetShowdown, Players: []user.TableSeat{{UserID: bob, Stack: 18, Put: 5}}}),
		},
		Stakes: []user.Payout{{UserID: ann, Votes: 13}},
		Rockets: []*user.RocketRound{
			user.RestoreRocketRound(uuid.New(), ann, 9, 1000, 0, at, 0, 0),
			user.RestoreRocketRound(uuid.New(), ann, 200, 100, 0, at, 0, 0),
		},
		Poker:  []user.Payout{{UserID: bob, Votes: 11}},
		Mine:   user.DefaultMine(),
		Miners: []user.ClosingMiner{{UserID: ann, Backpack: map[user.Ore]int64{user.OreStone: 150}}},
		Shop:   user.DefaultPetShop(),
		Pets: []user.ClosingPet{{UserID: ann, Kind: user.PetCat, Wardrobe: []user.PetItem{"bow"}},
			{UserID: ann, Kind: user.PetBig2Z}},
	}
}
