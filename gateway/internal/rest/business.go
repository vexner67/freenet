package rest

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Business is the businesses of the pages, which the user service keeps
// with the votes: people buy and run them, the administrators tune them.
type Business interface {
	Get(ctx context.Context, accessToken string) (model.Businesses, error)
	UserBusinesses(ctx context.Context, accessToken, nickname string) ([]model.UserBusiness, error)
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
	Staff   int    `json:"staff"`
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
	Wages int                `json:"wages"`
	Goods int                `json:"goods"`
}

type businessCatalogJSON struct {
	Kind        string                       `json:"kind"`
	Supplies    []businessSupplySpecJSON     `json:"supplies"`
	Offers      []businessOfferSpecJSON      `json:"offers"`
	Professions []businessProfessionSpecJSON `json:"professions"`
}

type businessSupplySpecJSON struct {
	Supply  string `json:"supply"`
	Cost    int64  `json:"cost"`
	Spoil   int    `json:"spoil"`
	Storage int64  `json:"storage"`
}

type businessOfferSpecJSON struct {
	Offer      string                   `json:"offer"`
	Profession string                   `json:"profession"`
	Recipe     []businessIngredientJSON `json:"recipe"`
	Fair       int64                    `json:"fair"`
	Demand     float64                  `json:"demand"`
}

type businessIngredientJSON struct {
	Supply string `json:"supply"`
	Units  int64  `json:"units"`
}

type businessProfessionSpecJSON struct {
	Profession string  `json:"profession"`
	Wage       int64   `json:"wage"`
	Serve      float64 `json:"serve"`
}

type businessSupplierJSON struct {
	Supplier string  `json:"supplier"`
	Percent  int64   `json:"percent"`
	Hours    float64 `json:"hours"`
	Short    int     `json:"short"`
}

type businessJSON struct {
	Kind            string                  `json:"kind"`
	Level           int                     `json:"level"`
	Condition       int                     `json:"condition"`
	Cash            int64                   `json:"cash"`
	AdUntil         *time.Time              `json:"adUntil"`
	Bankrupt        bool                    `json:"bankrupt"`
	Invested        int64                   `json:"invested"`
	Collected       int64                   `json:"collected"`
	Resale          int64                   `json:"resale"`
	InDebt          bool                    `json:"inDebt"`
	Supplies        []businessStockJSON     `json:"supplies"`
	Offers          []businessOfferJSON     `json:"offers"`
	Workers         []businessWorkerJSON    `json:"workers"`
	Candidates      []businessCandidateJSON `json:"candidates"`
	CandidatesUntil *time.Time              `json:"candidatesUntil"`
	Orders          []businessOrderJSON     `json:"orders"`
	Contracts       []businessContractJSON  `json:"contracts"`
	Event           *businessEventJSON      `json:"event"`
	Notes           []businessNoteJSON      `json:"notes"`
	Hours           []businessHourJSON      `json:"hours"`
	Seats           int                     `json:"seats"`
	ReviewUntil     *time.Time              `json:"reviewUntil"`
	Tax             int                     `json:"tax"`
	Wages           float64                 `json:"wages"`
	Rent            float64                 `json:"rent"`
	Till            int64                   `json:"till"`
	DebtLimit       int64                   `json:"debtLimit"`
	Load            map[string]float64      `json:"load"`
	Broken          string                  `json:"broken"`
	BrokenUntil     *time.Time              `json:"brokenUntil"`
	UpgradeCost     int64                   `json:"upgradeCost"`
	RepairCost      int64                   `json:"repairCost"`
}

type businessStockJSON struct {
	Supply    string             `json:"supply"`
	Units     float64            `json:"units"`
	OnWay     int64              `json:"onWay"`
	Storage   int64              `json:"storage"`
	UnitCosts map[string]float64 `json:"unitCosts"`
	Use       float64            `json:"use"`
}

type businessOfferJSON struct {
	Offer    string  `json:"offer"`
	Price    int64   `json:"price"`
	Demand   float64 `json:"demand"`
	Served   float64 `json:"served"`
	Capacity float64 `json:"capacity"`
	Broken   bool    `json:"broken"`
}

type businessWorkerJSON struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Female     bool    `json:"female"`
	Profession string  `json:"profession"`
	Skill      int     `json:"skill"`
	XP         float64 `json:"xp"`
	NextXP     float64 `json:"nextXp"`
	Wage       int64   `json:"wage"`
	Trait      string  `json:"trait"`
	Mood       float64 `json:"mood"`
	Rate       float64 `json:"rate"`
	Bonus      int64   `json:"bonus"`
}

type businessCandidateJSON struct {
	// A string: the ID does not fit the numbers of JavaScript.
	ID     string             `json:"id"`
	Worker businessWorkerJSON `json:"worker"`
}

type businessOrderJSON struct {
	ID       int       `json:"id"`
	Supply   string    `json:"supply"`
	Supplier string    `json:"supplier"`
	Units    int64     `json:"units"`
	Arrives  time.Time `json:"arrives"`
	FromTill bool      `json:"fromTill"`
}

type businessContractJSON struct {
	Supply   string `json:"supply"`
	Supplier string `json:"supplier"`
	Below    int64  `json:"below"`
	Units    int64  `json:"units"`
	Stalled  bool   `json:"stalled"`
}

type businessEventJSON struct {
	Kind    string    `json:"kind"`
	Until   time.Time `json:"until"`
	Offer   string    `json:"offer"`
	Worker  int       `json:"worker"`
	Name    string    `json:"name"`
	Female  bool      `json:"female"`
	Votes   int64     `json:"votes"`
	Units   int64     `json:"units"`
	Pay     int64     `json:"pay"`
	Options []string  `json:"options"`
}

type businessNoteJSON struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Subject string    `json:"subject"`
	Name    string    `json:"name"`
	Female  bool      `json:"female"`
	Amount  float64   `json:"amount"`
	Money   float64   `json:"money"`
}

type businessHourJSON struct {
	Hour     time.Time          `json:"hour"`
	Sold     map[string]float64 `json:"sold"`
	Income   float64            `json:"income"`
	Wages    float64            `json:"wages"`
	Rent     float64            `json:"rent"`
	Supplies float64            `json:"supplies"`
	Losses   float64            `json:"losses"`
	Spoiled  float64            `json:"spoiled"`
	Unserved float64            `json:"unserved"`
}

type businessesJSON struct {
	Settings   businessSettingsJSON   `json:"settings"`
	Businesses []businessJSON         `json:"businesses"`
	Catalog    []businessCatalogJSON  `json:"catalog"`
	Suppliers  []businessSupplierJSON `json:"suppliers"`
}

// listJSON is a list in JSON, empty rather than null.
func listJSON[T, U any](items []T, to func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, to(item))
	}

	return out
}

// mapJSON is a map in JSON, empty rather than null.
func mapJSON(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}

	return m
}

// timeJSON is a time in JSON, null when zero.
func timeJSON(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}

func toBusinessSettingsJSON(s model.BusinessSettings) businessSettingsJSON {
	return businessSettingsJSON{
		Plans: listJSON(s.Plans, func(p model.BusinessPlan) businessPlanJSON { return businessPlanJSON(p) }),
		Tax:   s.Tax, Wages: s.Wages, Goods: s.Goods,
	}
}

func toBusinessCatalogJSON(c model.BusinessCatalog) businessCatalogJSON {
	return businessCatalogJSON{
		Kind: c.Kind,
		Supplies: listJSON(c.Supplies, func(s model.BusinessSupplySpec) businessSupplySpecJSON {
			return businessSupplySpecJSON(s)
		}),
		Offers: listJSON(c.Offers, func(o model.BusinessOfferSpec) businessOfferSpecJSON {
			return businessOfferSpecJSON{
				Offer: o.Offer, Profession: o.Profession, Fair: o.Fair, Demand: o.Demand,
				Recipe: listJSON(o.Recipe, func(i model.BusinessIngredient) businessIngredientJSON {
					return businessIngredientJSON(i)
				}),
			}
		}),
		Professions: listJSON(c.Professions, func(p model.BusinessProfessionSpec) businessProfessionSpecJSON {
			return businessProfessionSpecJSON(p)
		}),
	}
}

func toBusinessJSON(b model.Business) businessJSON {
	out := businessJSON{
		Kind: b.Kind, Level: b.Level, Condition: b.Condition, Cash: b.Cash, AdUntil: timeJSON(b.AdUntil),
		Bankrupt: b.Bankrupt, Invested: b.Invested, Collected: b.Collected, Resale: b.Resale, InDebt: b.InDebt,
		CandidatesUntil: timeJSON(b.CandidatesUntil), Seats: b.Seats, ReviewUntil: timeJSON(b.ReviewUntil),
		Tax: b.Tax, Wages: b.Wages, Rent: b.Rent, Till: b.Till, DebtLimit: b.DebtLimit, Load: mapJSON(b.Load),
		Broken: b.Broken, BrokenUntil: timeJSON(b.BrokenUntil), UpgradeCost: b.UpgradeCost, RepairCost: b.RepairCost,
		Supplies: listJSON(b.Supplies, func(s model.BusinessStock) businessStockJSON {
			s.UnitCosts = mapJSON(s.UnitCosts)
			return businessStockJSON(s)
		}),
		Offers:  listJSON(b.Offers, func(o model.BusinessOffer) businessOfferJSON { return businessOfferJSON(o) }),
		Workers: listJSON(b.Workers, func(w model.BusinessWorker) businessWorkerJSON { return businessWorkerJSON(w) }),
		Candidates: listJSON(b.Candidates, func(c model.BusinessCandidate) businessCandidateJSON {
			return businessCandidateJSON{ID: strconv.FormatInt(c.ID, 10), Worker: businessWorkerJSON(c.Worker)}
		}),
		Orders:    listJSON(b.Orders, func(o model.BusinessOrder) businessOrderJSON { return businessOrderJSON(o) }),
		Contracts: listJSON(b.Contracts, func(c model.BusinessContract) businessContractJSON { return businessContractJSON(c) }),
		Notes:     listJSON(b.Notes, func(n model.BusinessNote) businessNoteJSON { return businessNoteJSON(n) }),
		Hours: listJSON(b.Hours, func(h model.BusinessHour) businessHourJSON {
			h.Sold = mapJSON(h.Sold)
			return businessHourJSON(h)
		}),
	}
	if b.Event != nil {
		e := businessEventJSON(*b.Event)
		if e.Options == nil {
			e.Options = []string{}
		}

		out.Event = &e
	}

	return out
}

func toBusinessesJSON(b model.Businesses) businessesJSON {
	return businessesJSON{
		Settings:   toBusinessSettingsJSON(b.Settings),
		Businesses: listJSON(b.Businesses, toBusinessJSON),
		Catalog:    listJSON(b.Catalog, toBusinessCatalogJSON),
		Suppliers:  listJSON(b.Suppliers, func(s model.BusinessSupplier) businessSupplierJSON { return businessSupplierJSON(s) }),
	}
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
		Amount    int64  `json:"amount"`
		Price     int64  `json:"price"`
		Offer     string `json:"offer"`
		Supply    string `json:"supply"`
		Supplier  string `json:"supplier"`
		Below     int64  `json:"below"`
		Worker    int    `json:"worker"`
		Candidate string `json:"candidate"`
		Option    string `json:"option"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	var candidate int64
	if req.Candidate != "" {
		if candidate, err = strconv.ParseInt(req.Candidate, 10, 64); err != nil {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid candidate"))
			return
		}
	}

	d, err := a.business.Run(r.Context(), token, model.BusinessAct{
		Kind: r.PathValue("kind"), Action: r.PathValue("action"), Amount: req.Amount, Price: req.Price,
		Offer: req.Offer, Supply: req.Supply, Supplier: req.Supplier, Below: req.Below, Worker: req.Worker,
		Candidate: candidate, Option: req.Option,
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

	settings := model.BusinessSettings{Tax: req.Tax, Wages: req.Wages, Goods: req.Goods}
	for _, p := range req.Plans {
		settings.Plans = append(settings.Plans, model.BusinessPlan(p))
	}

	if err = a.business.Set(r.Context(), token, settings); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type userBusinessJSON struct {
	Kind     string `json:"kind"`
	Level    int    `json:"level"`
	Workers  int    `json:"workers"`
	Bankrupt bool   `json:"bankrupt"`
}

// getUserBusinesses are the businesses of a page, public as the page.
func (a *api) getUserBusinesses(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owned, err := a.business.UserBusinesses(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Businesses []userBusinessJSON `json:"businesses"`
	}{listJSON(owned, func(b model.UserBusiness) userBusinessJSON {
		return userBusinessJSON{Kind: b.Kind, Level: b.Level, Workers: b.Workers, Bankrupt: b.Bankrupt}
	})})
}
