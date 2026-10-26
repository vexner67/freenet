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

type fakeBusiness struct {
	token    string
	nickname string
	kind     string
	act      model.BusinessAct
	set      model.BusinessSettings
	err      error
}

func (f *fakeBusiness) businesses() model.Businesses {
	return model.Businesses{
		Settings: model.BusinessSettings{Plans: []model.BusinessPlan{{Kind: "kiosk", Price: 500, AdHours: 24}}, Tax: 5, Wages: 100, Goods: 100},
		Businesses: []model.Business{{
			Kind: "kiosk", Level: 2, Condition: 9990, Cash: -150, InDebt: true,
			AdUntil:    time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
			Supplies:   []model.BusinessStock{{Supply: "drinks", Units: 40, Storage: 200}},
			Candidates: []model.BusinessCandidate{{ID: 9007199254740993, Worker: model.BusinessWorker{Name: "Анна", Profession: "seller"}}},
			Event:      &model.BusinessEvent{Kind: "inspection", Options: []string{"pay", "refuse"}},
		}},
		Catalog:   []model.BusinessCatalog{{Kind: "kiosk", Offers: []model.BusinessOfferSpec{{Offer: "drink", Recipe: []model.BusinessIngredient{{Supply: "drinks", Units: 1}}}}}},
		Suppliers: []model.BusinessSupplier{{Supplier: "reliable", Percent: 100, Hours: 2}},
	}
}

func (f *fakeBusiness) Get(_ context.Context, token string) (model.Businesses, error) {
	f.token = token
	return f.businesses(), f.err
}

func (f *fakeBusiness) UserBusinesses(_ context.Context, token, nickname string) ([]model.UserBusiness, error) {
	f.token, f.nickname = token, nickname
	return []model.UserBusiness{{Kind: "kiosk", Level: 2, Workers: 3, Bankrupt: true}}, f.err
}

func (f *fakeBusiness) Buy(_ context.Context, token, kind string) (model.BusinessDone, error) {
	f.token, f.kind = token, kind
	return model.BusinessDone{Spent: 500, Balance: 100, Businesses: f.businesses()}, f.err
}

func (f *fakeBusiness) Run(_ context.Context, token string, act model.BusinessAct) (model.BusinessDone, error) {
	f.token, f.act = token, act
	return model.BusinessDone{Got: 19, Tax: 1, Balance: 119, Businesses: f.businesses()}, f.err
}

func (f *fakeBusiness) Settings(_ context.Context, token string) (model.BusinessSettings, model.BusinessStats, error) {
	f.token = token
	return f.businesses().Settings, model.BusinessStats{Businesses: 3, Spent: 1500, Paid: 40}, f.err
}

func (f *fakeBusiness) Set(_ context.Context, token string, settings model.BusinessSettings) error {
	f.token, f.set = token, settings
	return f.err
}

func TestBusiness(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("GET", "/api/v1/me/businesses", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("guest: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/me/businesses", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	biz := got["businesses"].([]any)[0].(map[string]any)
	if w.Code != http.StatusOK || h.business.token != "tok" || biz["cash"] != -150.0 || biz["inDebt"] != true ||
		biz["adUntil"] != "2026-10-25T12:00:00Z" || biz["reviewUntil"] != nil || got["settings"].(map[string]any)["tax"] != 5.0 ||
		biz["candidates"].([]any)[0].(map[string]any)["id"] != "9007199254740993" || biz["orders"] == nil ||
		biz["event"].(map[string]any)["options"].([]any)[1] != "refuse" || len(got["catalog"].([]any)) != 1 ||
		got["suppliers"].([]any)[0].(map[string]any)["hours"] != 2.0 {
		t.Fatalf("businesses: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/businesses", `{"kind":"kiosk"}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || h.business.kind != "kiosk" || got["spent"] != 500.0 {
		t.Fatalf("buy: %d %v", w.Code, got)
	}

	w = h.do("POST", "/api/v1/me/businesses/kiosk/order", `{"supply":"drinks","supplier":"reliable","amount":100}`, "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK ||
		h.business.act != (model.BusinessAct{Kind: "kiosk", Action: "order", Supply: "drinks", Supplier: "reliable", Amount: 100}) ||
		got["balance"] != 119.0 {
		t.Fatalf("order: %d %v %+v", w.Code, got, h.business.act)
	}

	w = h.do("POST", "/api/v1/me/businesses/kiosk/hire", `{"candidate":"9007199254740993"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.business.act.Candidate != 9007199254740993 {
		t.Fatalf("hire: %d %+v", w.Code, h.business.act)
	}
	if w = h.do("POST", "/api/v1/me/businesses/kiosk/hire", `{"candidate":"anna"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("hire of a word: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/business/settings", "", "Authorization", "Bearer tok")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["businesses"] != 3.0 || got["paid"] != 40.0 {
		t.Fatalf("settings: %d %v", w.Code, got)
	}

	w = h.do("PUT", "/api/v1/business/settings", `{"plans":[{"kind":"kiosk","price":7,"adBoost":30}],"tax":9,"wages":120,"goods":80}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.business.set.Tax != 9 || h.business.set.Wages != 120 || h.business.set.Goods != 80 || h.business.set.Plans[0].Price != 7 || h.business.set.Plans[0].AdBoost != 30 {
		t.Fatalf("set: %d %+v", w.Code, h.business.set)
	}

	h.business.err = failure.New(failure.Conflict, "the business went bankrupt")
	if w = h.do("POST", "/api/v1/me/businesses/kiosk/upgrade", `{}`, "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("bankrupt: %d", w.Code)
	}
}

func TestUserBusinesses(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/ann/businesses", "")
	got := decode[map[string]any](t, w)["businesses"].([]any)
	if w.Code != http.StatusOK || h.business.nickname != "ann" || h.business.token != "" || len(got) != 1 ||
		got[0].(map[string]any)["kind"] != "kiosk" || got[0].(map[string]any)["workers"] != 3.0 || got[0].(map[string]any)["bankrupt"] != true {
		t.Fatalf("businesses of a page: %d %v", w.Code, got)
	}

	h.business.err = failure.New(failure.NotFound, "user not found")
	if w = h.do("GET", "/api/v1/users/gone/businesses", ""); w.Code != http.StatusNotFound {
		t.Fatalf("businesses of a deleted page: %d", w.Code)
	}
}
