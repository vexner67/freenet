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
		}},
		Orders:   []model.ExchangeOrder{{ID: "o1", FirmID: "f1", Good: "grain", Price: 22, Units: 100}},
		Offers:   []model.ShareOffer{{ID: "s1", FirmID: "f1", SellerID: "u2", Shares: 10, Price: 9}, {ID: "s2", FirmID: "f1", Shares: 50, Price: 5}},
		NextHour: time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC),
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
	offers := got["offers"].([]any)
	if w.Code != http.StatusOK || h.firms.token != "tok" || firm["cash"] != 12345.0 || firm["name"] != "Колхоз" ||
		firm["director"].(map[string]any)["nickname"] != "ann" || len(firm["holders"].([]any)) != 2 ||
		firm["foundedAt"] != "2026-10-25T12:00:00Z" || len(firm["hours"].([]any)) != 0 ||
		offers[0].(map[string]any)["seller"].(map[string]any)["nickname"] != "bob" || offers[1].(map[string]any)["seller"] != nil ||
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

	w = h.do("PUT", "/api/v1/firm/settings", `{"plans":[{"kind":"farm","price":7,"staff":12}],"goods":[{"good":"grain","base":30,"norm":100}],"fee":3,"floor":70}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.firms.set.Fee != 3 || h.firms.set.Floor != 70 || h.firms.set.Plans[0].Staff != 12 || h.firms.set.Goods[0].Norm != 100 {
		t.Fatalf("set: %d %+v", w.Code, h.firms.set)
	}

	h.firms.err = failure.New(failure.Conflict, "the enterprise has not enough money")
	if w = h.do("POST", "/api/v1/firms/upgrade", `{"firm":"f1"}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
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
