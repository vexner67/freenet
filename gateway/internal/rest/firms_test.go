package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeFirms struct {
	token    string
	nickname string
	act      model.FirmAct
	stockAct model.StockAct
	firmID   string
	set      model.FirmSettings
	err      error
}

func (f *fakeFirms) city() model.Firms {
	return model.Firms{
		Settings: model.FirmSettings{Plans: []model.FirmPlan{{Kind: "farm", Price: 1000, Levels: 10}}, Wage: 50, Fee: 2},
		Catalog:  []model.FirmSpec{{Kind: "mill", Tier: 2, Output: "flour", Inputs: []model.FirmInput{{Good: "grain", Units: 2}}, Rate: map[string]float64{"flour": 5}}},
		Quotes:   []model.GoodQuote{{Good: "grain", StateBid: 20, StateAsk: 30}},
		Firms: []model.Firm{{
			ID: "f1", Kind: "farm", Name: "Колхоз", DirectorID: "u1", Level: 2, Cash: 12345,
			FoundedAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
			Stock:     []model.FirmStock{{Good: "grain", Units: 40, Capacity: 7200, AutoSell: true}},
			Holders:   []model.FirmHolder{{UserID: "u1", Shares: 60}, {UserID: "u2", Shares: 40}, {UserID: "u3", Shares: 1}},
			Wear:      12.5, RepairCost: 1250, Improvements: []model.FirmImprovement{{Kind: "machines", Rank: 1, Cost: 100000}},
		}},
		Orders:    []model.ExchangeOrder{{ID: "o1", FirmID: "f1", Good: "grain", Price: 22, Units: 100}},
		Contracts: []model.FirmContract{{ID: "k1", Good: "grain", Units: 800, Price: 32, FirmID: "f1"}},
		News:      []model.FirmEvent{{ID: "e1", Kind: "drought", Kinds: []string{"farm"}, Start: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}},
		NextHour:  time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC),
	}
}

func (f *fakeFirms) Get(_ context.Context, token string) (model.Firms, error) {
	f.token = token
	return f.city(), f.err
}

func (f *fakeFirms) UserFirms(_ context.Context, token, nickname string) ([]model.UserFirm, error) {
	f.token, f.nickname = token, nickname
	return []model.UserFirm{{ID: "f1", Name: "Колхоз", Kind: "farm", Level: 2, Shares: 60, Total: 100, Director: true}}, f.err
}

func (f *fakeFirms) Run(_ context.Context, token string, act model.FirmAct) (model.FirmDone, error) {
	f.token, f.act = token, act
	return model.FirmDone{Spent: 1000, Balance: 119, FirmID: "f1", Deals: []model.FirmDeal{{Good: "grain", Units: 5, Price: 22}}, Firms: f.city()}, f.err
}

func (f *fakeFirms) exchange() model.Stocks {
	return model.Stocks{
		Companies: []model.StockCompany{{
			FirmID: "f1", Name: "Колхоз", Kind: "farm", DirectorID: "u1", Shares: 100, Price: 1050, Bid: 990, Ask: 1100,
			Candles: []model.StockCandle{{Hour: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC), Open: 1000, Close: 1050}},
		}},
		Index:    []model.StockIndexPoint{{Hour: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC), Value: 100000}},
		Fee:      2,
		Account:  50000,
		Holdings: []model.StockHolding{{FirmID: "f1", Shares: 10, Free: 4}},
		Orders:   []model.StockOrder{{ID: "o1", FirmID: "f1", Price: 1200, Shares: 6}},
		FirmID:   f.firmID,
		Asks:     []model.StockLevel{{Price: 1200, Shares: 6, Orders: 1}},
	}
}

func (f *fakeFirms) Stocks(_ context.Context, token, firmID string) (model.Stocks, error) {
	f.token, f.firmID = token, firmID
	return f.exchange(), f.err
}

func (f *fakeFirms) RunStocks(_ context.Context, token string, act model.StockAct) (model.StockDone, error) {
	f.token, f.stockAct, f.firmID = token, act, act.FirmID
	return model.StockDone{Shares: 3, Money: 3150, Stocks: f.exchange()}, f.err
}

func (f *fakeFirms) Settings(_ context.Context, token string) (model.FirmSettings, model.FirmStats, error) {
	f.token = token
	return f.city().Settings, model.FirmStats{Firms: 3, Spent: 1500, Paid: 40, Fees: 7}, f.err
}

func (f *fakeFirms) Set(_ context.Context, token string, settings model.FirmSettings) error {
	f.token, f.set = token, settings
	return f.err
}

func TestFirms(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("GET", "/api/v1/firms", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("guest: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/firms", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	firm := got["firms"].([]any)[0].(map[string]any)
	contract := got["contracts"].([]any)[0].(map[string]any)
	news := got["news"].([]any)[0].(map[string]any)
	if w.Code != http.StatusOK || h.firms.token != "tok" || firm["cash"] != 12345.0 || firm["name"] != "Колхоз" ||
		firm["director"].(map[string]any)["nickname"] != "ann" || len(firm["holders"].([]any)) != 2 ||
		firm["foundedAt"] != "2026-10-25T12:00:00Z" || len(firm["hours"].([]any)) != 0 ||
		firm["wear"] != 12.5 || firm["repairCost"] != 1250.0 || firm["idleUntil"] != "" ||
		firm["improvements"].([]any)[0].(map[string]any)["cost"] != 100000.0 ||
		contract["firm"] != "f1" || contract["units"] != 800.0 || news["kind"] != "drought" || news["kinds"].([]any)[0] != "farm" ||
		got["settings"].(map[string]any)["fee"] != 2.0 || got["nextHour"] != "2026-10-25T13:00:00Z" ||
		got["catalog"].([]any)[0].(map[string]any)["inputs"].([]any)[0].(map[string]any)["units"] != 2.0 {
		t.Fatalf("firms: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/firms/order", `{"firm":"f1","good":"grain","buy":true,"price":22,"units":5}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK ||
		h.firms.act != (model.FirmAct{Action: "order", FirmID: "f1", Good: "grain", Buy: true, Price: 22, Units: 5}) ||
		got["balance"] != 119.0 || got["firm"] != "f1" || got["deals"].([]any)[0].(map[string]any)["price"] != 22.0 {
		t.Fatalf("order: %d %v %+v", w.Code, got, h.firms.act)
	}

	w = h.do("POST", "/api/v1/firms/auto", `{"firm":"f1","good":"grain","autoSell":true,"sellMin":15}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || !h.firms.act.AutoSell || h.firms.act.SellMin != 15 {
		t.Fatalf("auto: %d %+v", w.Code, h.firms.act)
	}

	w = h.do("GET", "/api/v1/firm/settings", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["firms"] != 3.0 || got["fees"] != 7.0 {
		t.Fatalf("settings: %d %v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/firm/settings", `{"plans":[{"kind":"farm","price":7,"staff":12}],"goods":[{"good":"grain","base":30,"norm":100}],"fee":3,"floor":70,"maxFirms":4,"foundingShares":200,"floatShares":0,"maxShares":5000}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.firms.set.Fee != 3 || h.firms.set.Floor != 70 || h.firms.set.Plans[0].Staff != 12 || h.firms.set.Goods[0].Norm != 100 ||
		h.firms.set.MaxFirms != 4 || h.firms.set.FoundingShares != 200 || h.firms.set.MaxShares != 5000 {
		t.Fatalf("set: %d %+v", w.Code, h.firms.set)
	}

	h.firms.err = failure.New(failure.Conflict, "the enterprise has not enough money")
	if w = h.do("POST", "/api/v1/firms/upgrade", `{"firm":"f1"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("no money: %d", w.Code)
	}
}

func TestStocks(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("GET", "/api/v1/stocks", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("guest: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/stocks?firm=f1", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	company := got["companies"].([]any)[0].(map[string]any)
	if w.Code != http.StatusOK || h.firms.token != "tok" || h.firms.firmID != "f1" || got["firm"] != "f1" ||
		company["director"].(map[string]any)["nickname"] != "ann" || company["price"] != 1050.0 ||
		company["candles"].([]any)[0].(map[string]any)["hour"] != "2026-10-25T12:00:00Z" || got["account"] != 50000.0 ||
		got["holdings"].([]any)[0].(map[string]any)["free"] != 4.0 || got["asks"].([]any)[0].(map[string]any)["shares"] != 6.0 ||
		len(got["bids"].([]any)) != 0 || got["index"].([]any)[0].(map[string]any)["value"] != 100000.0 {
		t.Fatalf("stocks: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/stocks/stock_order", `{"firm":"f1","buy":true,"price":1100,"shares":3}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK ||
		h.firms.stockAct != (model.StockAct{Action: "stock_order", FirmID: "f1", Buy: true, Price: 1100, Shares: 3}) ||
		got["shares"] != 3.0 || got["money"] != 3150.0 || got["stocks"].(map[string]any)["firm"] != "f1" {
		t.Fatalf("order: %d %v %+v", w.Code, got, h.firms.stockAct)
	}

	h.firms.err = failure.New(failure.Conflict, "there is not so much money on your account")
	if w = h.do("POST", "/api/v1/stocks/account_withdraw", `{"votes":10}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("no money: %d", w.Code)
	}
}

func TestUserFirms(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/ann/firms", "")
	got := decode[map[string]any](t, w)["firms"].([]any)
	if w.Code != http.StatusOK || h.firms.nickname != "ann" || h.firms.token != "" || len(got) != 1 ||
		got[0].(map[string]any)["name"] != "Колхоз" || got[0].(map[string]any)["director"] != true || got[0].(map[string]any)["total"] != 100.0 {
		t.Fatalf("firms of a page: %d %v", w.Code, got)
	}

	h.firms.err = failure.New(failure.NotFound, "user not found")
	if w = h.do("GET", "/api/v1/users/gone/firms", ""); w.Code != http.StatusNotFound {
		t.Fatalf("firms of a deleted page: %d", w.Code)
	}
}
