package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Business is the businesses of the pages, which the user service keeps
// with the votes: people buy and run them, the administrators tune them.
type Business interface {
	Get(ctx context.Context, accessToken string) (model.Businesses, error)
	Buy(ctx context.Context, accessToken, kind string) (model.BusinessDone, error)
	Run(ctx context.Context, accessToken string, act model.BusinessAct) (model.BusinessDone, error)
	Settings(ctx context.Context, accessToken string) (model.BusinessSettings, model.BusinessStats, error)
	Set(ctx context.Context, accessToken string, settings model.BusinessSettings) error
}

type businessPlanJSON struct {
	Kind    string `json:"kind"`
	Price   int64  `json:"price"`
	Upgrade int64  `json:"upgrade"`
	Levels  int    `json:"levels"`
	Demand  int64  `json:"demand"`
	Fair    int64  `json:"fair"`
	Cost    int64  `json:"cost"`
	Storage int64  `json:"storage"`
	Serve   int64  `json:"serve"`
	Staff   int    `json:"staff"`
	Wage    int64  `json:"wage"`
	Rent    int64  `json:"rent"`
	Wear    int    `json:"wear"`
	Repair  int64  `json:"repair"`
	Till    int64  `json:"till"`
	Debt    int64  `json:"debt"`
	AdPrice int64  `json:"adPrice"`
	AdHours int    `json:"adHours"`
	AdBoost int    `json:"adBoost"`
}

type businessSettingsJSON struct {
	Plans []businessPlanJSON `json:"plans"`
	Tax   int                `json:"tax"`
}

type businessJSON struct {
	Kind      string             `json:"kind"`
	Level     int                `json:"level"`
	Stock     int64              `json:"stock"`
	Price     int64              `json:"price"`
	Staff     int                `json:"staff"`
	Condition int                `json:"condition"`
	Cash      int64              `json:"cash"`
	AdUntil   *time.Time         `json:"adUntil"`
	Bankrupt  bool               `json:"bankrupt"`
	Invested  int64              `json:"invested"`
	Collected int64              `json:"collected"`
	Customers float64            `json:"customers"`
	Served    float64            `json:"served"`
	Income    float64            `json:"income"`
	Costs     float64            `json:"costs"`
	Resale    int64              `json:"resale"`
	InDebt    bool               `json:"inDebt"`
	Perks     []businessPerkJSON `json:"perks"`
	Storage   int64              `json:"storage"`
}

type businessPerkJSON struct {
	Perk      string `json:"perk"`
	Tier      int    `json:"tier"`
	NextPrice int64  `json:"nextPrice"`
	NextLevel int    `json:"nextLevel"`
}

type businessesJSON struct {
	Settings   businessSettingsJSON `json:"settings"`
	Businesses []businessJSON       `json:"businesses"`
}

func toBusinessSettingsJSON(s model.BusinessSettings) businessSettingsJSON {
	out := businessSettingsJSON{Plans: make([]businessPlanJSON, 0, len(s.Plans)), Tax: s.Tax}
	for _, p := range s.Plans {
		out.Plans = append(out.Plans, businessPlanJSON(p))
	}

	return out
}

func toBusinessesJSON(b model.Businesses) businessesJSON {
	out := businessesJSON{Settings: toBusinessSettingsJSON(b.Settings), Businesses: make([]businessJSON, 0, len(b.Businesses))}
	for _, biz := range b.Businesses {
		j := businessJSON{
			Kind: biz.Kind, Level: biz.Level, Stock: biz.Stock, Price: biz.Price, Staff: biz.Staff,
			Condition: biz.Condition, Cash: biz.Cash, Bankrupt: biz.Bankrupt, Invested: biz.Invested,
			Collected: biz.Collected, Customers: biz.Customers, Served: biz.Served, Income: biz.Income,
			Costs: biz.Costs, Resale: biz.Resale, InDebt: biz.InDebt, Storage: biz.Storage,
			Perks: make([]businessPerkJSON, 0, len(biz.Perks)),
		}
		for _, p := range biz.Perks {
			j.Perks = append(j.Perks, businessPerkJSON(p))
		}
		if !biz.AdUntil.IsZero() {
			until := biz.AdUntil
			j.AdUntil = &until
		}

		out.Businesses = append(out.Businesses, j)
	}

	return out
}

func writeBusinessDone(w http.ResponseWriter, d model.BusinessDone) {
	writeJSON(w, http.StatusOK, struct {
		Spent      int64          `json:"spent"`
		Got        int64          `json:"got"`
		Tax        int64          `json:"tax"`
		Balance    int64          `json:"balance"`
		Businesses businessesJSON `json:"businesses"`
	}{d.Spent, d.Got, d.Tax, d.Balance, toBusinessesJSON(d.Businesses)})
}

// businesses is "Бизнес" of the caller: the plans and the businesses.
func (a *api) businesses(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	b, err := a.business.Get(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toBusinessesJSON(b))
}

func (a *api) buyBusiness(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Kind string `json:"kind"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	d, err := a.business.Buy(r.Context(), token, req.Kind)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeBusinessDone(w, d)
}

// runBusiness does an action with a business of the caller.
func (a *api) runBusiness(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Amount int64  `json:"amount"`
		Price  int64  `json:"price"`
		Staff  int    `json:"staff"`
		Perk   string `json:"perk"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	d, err := a.business.Run(r.Context(), token, model.BusinessAct{
		Kind: r.PathValue("kind"), Action: r.PathValue("action"),
		Amount: req.Amount, Price: req.Price, Staff: req.Staff, Perk: req.Perk,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeBusinessDone(w, d)
}

// businessSettings is "Бизнес" of the admin panel: the plans and how the
// businesses do.
func (a *api) businessSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	s, stats, err := a.business.Settings(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Settings   businessSettingsJSON `json:"settings"`
		Businesses int64                `json:"businesses"`
		Spent      int64                `json:"spent"`
		Paid       int64                `json:"paid"`
	}{toBusinessSettingsJSON(s), stats.Businesses, stats.Spent, stats.Paid})
}

func (a *api) setBusinessSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req businessSettingsJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	settings := model.BusinessSettings{Tax: req.Tax}
	for _, p := range req.Plans {
		settings.Plans = append(settings.Plans, model.BusinessPlan(p))
	}

	if err = a.business.Set(r.Context(), token, settings); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
