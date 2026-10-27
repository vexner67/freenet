package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Firms is "Бизнес", the economy of the enterprises, which the user
// service keeps with the votes: people found and run enterprises, trade
// and hold their shares on the stock exchange, the administrators tune
// them.
type Firms interface {
	Stocks(ctx context.Context, accessToken, firmID string) (model.Stocks, error)
	RunStocks(ctx context.Context, accessToken string, act model.StockAct) (model.StockDone, error)
	Get(ctx context.Context, accessToken string) (model.Firms, error)
	UserFirms(ctx context.Context, accessToken, nickname string) ([]model.UserFirm, error)
	Run(ctx context.Context, accessToken string, act model.FirmAct) (model.FirmDone, error)
	Settings(ctx context.Context, accessToken string) (model.FirmSettings, model.FirmStats, error)
	Set(ctx context.Context, accessToken string, settings model.FirmSettings) error
}

type firmPlanJSON struct {
	Kind    string `json:"kind"`
	Price   int64  `json:"price"`
	Upgrade int64  `json:"upgrade"`
	Levels  int    `json:"levels"`
	Staff   int    `json:"staff"`
	Upkeep  int64  `json:"upkeep"`
}

type goodPlanJSON struct {
	Good string `json:"good"`
	Base int64  `json:"base"`
	Norm int64  `json:"norm"`
}

type firmSettingsJSON struct {
	Plans   []firmPlanJSON `json:"plans"`
	Goods   []goodPlanJSON `json:"goods"`
	Wage    int64          `json:"wage"`
	Fee     int            `json:"fee"`
	Floor   int            `json:"floor"`
	Ceiling int            `json:"ceiling"`
	Retail  int            `json:"retail"`
	Storage int            `json:"storage"`
	Capital int            `json:"capital"`
	// MaxFirms is how many enterprises a page may direct; the shares are
	// the founder's, those sold at founding and the most an enterprise has.
	MaxFirms       int   `json:"maxFirms"`
	FoundingShares int64 `json:"foundingShares"`
	FloatShares    int64 `json:"floatShares"`
	MaxShares      int64 `json:"maxShares"`
}

type firmInputJSON struct {
	Good  string `json:"good"`
	Units int64  `json:"units"`
}

type firmSpecJSON struct {
	Kind   string             `json:"kind"`
	Tier   int                `json:"tier"`
	Output string             `json:"output"`
	Inputs []firmInputJSON    `json:"inputs"`
	Sells  []string           `json:"sells"`
	Rate   map[string]float64 `json:"rate"`
}

type goodQuoteJSON struct {
	Good      string `json:"good"`
	StateBid  int64  `json:"stateBid"`
	StateAsk  int64  `json:"stateAsk"`
	CityPrice int64  `json:"cityPrice"`
	BestBid   int64  `json:"bestBid"`
	BestAsk   int64  `json:"bestAsk"`
	Last      int64  `json:"last"`
	Volume    int64  `json:"volume"`
}

type firmStockJSON struct {
	Good     string `json:"good"`
	Units    int64  `json:"units"`
	Capacity int64  `json:"capacity"`
	Incoming int64  `json:"incoming"`
	AutoBuy  bool   `json:"autoBuy"`
	BuyMax   int64  `json:"buyMax"`
	AutoSell bool   `json:"autoSell"`
	SellMin  int64  `json:"sellMin"`
}

type firmHourJSON struct {
	Hour      string `json:"hour"`
	Made      int64  `json:"made"`
	Bought    int64  `json:"bought"`
	Sold      int64  `json:"sold"`
	Income    int64  `json:"income"`
	Purchases int64  `json:"purchases"`
	Wages     int64  `json:"wages"`
	Upkeep    int64  `json:"upkeep"`
	Fees      int64  `json:"fees"`
}

type firmHolderJSON struct {
	User   publicProfileResponse `json:"user"`
	Shares int64                 `json:"shares"`
}

type firmJSON struct {
	ID           string                 `json:"id"`
	Kind         string                 `json:"kind"`
	Name         string                 `json:"name"`
	Director     *publicProfileResponse `json:"director"`
	FoundedAt    string                 `json:"foundedAt"`
	Level        int                    `json:"level"`
	Workers      int                    `json:"workers"`
	Wage         int64                  `json:"wage"`
	Cash         int64                  `json:"cash"`
	Escrow       int64                  `json:"escrow"`
	Productivity float64                `json:"productivity"`
	Seats        int                    `json:"seats"`
	UpgradeCost  int64                  `json:"upgradeCost"`
	Upkeep       int64                  `json:"upkeep"`
	Profit       int64                  `json:"profit"`
	Shares       int64                  `json:"shares"`
	Worth        int64                  `json:"worth"`
	Stock        []firmStockJSON        `json:"stock"`
	Hours        []firmHourJSON         `json:"hours"`
	Holders      []firmHolderJSON       `json:"holders"`
	Wear         float64                `json:"wear"`
	RepairCost   int64                  `json:"repairCost"`
	IdleUntil    string                 `json:"idleUntil"`
	Improvements []firmImprovementJSON  `json:"improvements"`
	Price        int64                  `json:"price"`
	Float        int64                  `json:"float"`
	Forecast     *firmHourJSON          `json:"forecast"`
	ForecastCash int64                  `json:"forecastCash"`
}

type firmImprovementJSON struct {
	Kind string `json:"kind"`
	Rank int    `json:"rank"`
	Cost int64  `json:"cost"`
}

type exchangeOrderJSON struct {
	ID       string `json:"id"`
	FirmID   string `json:"firmId"`
	Good     string `json:"good"`
	Buy      bool   `json:"buy"`
	Price    int64  `json:"price"`
	Units    int64  `json:"units"`
	PlacedAt string `json:"placedAt"`
}

type firmContractJSON struct {
	ID        string `json:"id"`
	Good      string `json:"good"`
	Units     int64  `json:"units"`
	Price     int64  `json:"price"`
	Posted    string `json:"posted"`
	Deadline  string `json:"deadline"`
	Firm      string `json:"firm"`
	Delivered int64  `json:"delivered"`
}

type firmEventJSON struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"`
	Firm  string   `json:"firm"`
	Kinds []string `json:"kinds"`
	Start string   `json:"start"`
	End   string   `json:"end"`
}

func toFirmEventsJSON(news []model.FirmEvent) []firmEventJSON {
	return listJSON(news, func(e model.FirmEvent) firmEventJSON {
		return firmEventJSON{
			ID: e.ID, Kind: e.Kind, Firm: e.FirmID, Kinds: listJSON(e.Kinds, func(k string) string { return k }),
			Start: timeString(e.Start), End: timeString(e.End),
		}
	})
}

type firmsJSON struct {
	Settings  firmSettingsJSON    `json:"settings"`
	Catalog   []firmSpecJSON      `json:"catalog"`
	Quotes    []goodQuoteJSON     `json:"quotes"`
	Firms     []firmJSON          `json:"firms"`
	Orders    []exchangeOrderJSON `json:"orders"`
	Contracts []firmContractJSON  `json:"contracts"`
	News      []firmEventJSON     `json:"news"`
	NextHour  string              `json:"nextHour"`
}

func toFirmSettingsJSON(s model.FirmSettings) firmSettingsJSON {
	return firmSettingsJSON{
		Plans:          listJSON(s.Plans, func(p model.FirmPlan) firmPlanJSON { return firmPlanJSON(p) }),
		Goods:          listJSON(s.Goods, func(g model.GoodPlan) goodPlanJSON { return goodPlanJSON(g) }),
		Wage:           s.Wage,
		Fee:            s.Fee,
		Floor:          s.Floor,
		Ceiling:        s.Ceiling,
		Retail:         s.Retail,
		Storage:        s.Storage,
		Capital:        s.Capital,
		MaxFirms:       s.MaxFirms,
		FoundingShares: s.FoundingShares,
		FloatShares:    s.FloatShares,
		MaxShares:      s.MaxShares,
	}
}

// listJSON is a list in JSON, empty rather than null.
func listJSON[T, U any](items []T, to func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, to(item))
	}

	return out
}

// person is the profile of id among people, if the page is there to show.
func person(people map[string]model.PublicProfile, id string) *publicProfileResponse {
	p, ok := people[id]
	if !ok {
		return nil
	}

	profile := toPublicProfileResponse(p)

	return &profile
}

// firmsResponse is the city in JSON with the pages of its people.
func (a *api) firmsResponse(r *http.Request, f model.Firms) (firmsJSON, error) {
	var ids []string
	for _, firm := range f.Firms {
		ids = append(ids, firm.DirectorID)
		for _, h := range firm.Holders {
			ids = append(ids, h.UserID)
		}
	}

	people, err := a.profiles(r, ids)
	if err != nil {
		return firmsJSON{}, err
	}

	return firmsJSON{
		Settings: toFirmSettingsJSON(f.Settings),
		Catalog: listJSON(f.Catalog, func(c model.FirmSpec) firmSpecJSON {
			return firmSpecJSON{
				Kind: c.Kind, Tier: c.Tier, Output: c.Output, Sells: listJSON(c.Sells, func(s string) string { return s }), Rate: c.Rate,
				Inputs: listJSON(c.Inputs, func(in model.FirmInput) firmInputJSON { return firmInputJSON(in) }),
			}
		}),
		Quotes: listJSON(f.Quotes, func(q model.GoodQuote) goodQuoteJSON { return goodQuoteJSON(q) }),
		Firms:  listJSON(f.Firms, func(firm model.Firm) firmJSON { return toFirmJSON(firm, people) }),
		Orders: listJSON(f.Orders, func(o model.ExchangeOrder) exchangeOrderJSON {
			return exchangeOrderJSON{
				ID: o.ID, FirmID: o.FirmID, Good: o.Good, Buy: o.Buy, Price: o.Price, Units: o.Units, PlacedAt: timeString(o.PlacedAt),
			}
		}),
		Contracts: listJSON(f.Contracts, func(k model.FirmContract) firmContractJSON {
			return firmContractJSON{
				ID: k.ID, Good: k.Good, Units: k.Units, Price: k.Price, Posted: timeString(k.Posted),
				Deadline: timeString(k.Deadline), Firm: k.FirmID, Delivered: k.Delivered,
			}
		}),
		News:     toFirmEventsJSON(f.News),
		NextHour: timeString(f.NextHour),
	}, nil
}

func toFirmJSON(f model.Firm, people map[string]model.PublicProfile) firmJSON {
	out := firmJSON{
		ID: f.ID, Kind: f.Kind, Name: f.Name, Director: person(people, f.DirectorID), FoundedAt: timeString(f.FoundedAt),
		Level: f.Level, Workers: f.Workers, Wage: f.Wage, Cash: f.Cash, Escrow: f.Escrow, Productivity: f.Productivity,
		Seats: f.Seats, UpgradeCost: f.UpgradeCost, Upkeep: f.Upkeep, Profit: f.Profit, Shares: f.Shares,
		Worth: f.Worth, Wear: f.Wear, RepairCost: f.RepairCost, IdleUntil: timeString(f.IdleUntil),
		Improvements: listJSON(f.Improvements, func(i model.FirmImprovement) firmImprovementJSON { return firmImprovementJSON(i) }),
		Price:        f.Price, Float: f.Float,
		Stock:        listJSON(f.Stock, func(s model.FirmStock) firmStockJSON { return firmStockJSON(s) }),
		Hours:        listJSON(f.Hours, toFirmHourJSON),
		ForecastCash: f.ForecastCash,
		Holders:      []firmHolderJSON{},
	}
	if f.Forecast != nil {
		forecast := toFirmHourJSON(*f.Forecast)
		out.Forecast = &forecast
	}
	for _, h := range f.Holders {
		if p := person(people, h.UserID); p != nil {
			out.Holders = append(out.Holders, firmHolderJSON{User: *p, Shares: h.Shares})
		}
	}

	return out
}

// firms is "Бизнес": the whole city.
func (a *api) firms(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	f, err := a.firmsBackend.Get(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out, err := a.firmsResponse(r, f)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// runFirms does an action with the enterprises and answers with the city
// after it.
func (a *api) runFirms(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Firm     string `json:"firm"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		Good     string `json:"good"`
		Buy      bool   `json:"buy"`
		Price    int64  `json:"price"`
		Units    int64  `json:"units"`
		Workers  int    `json:"workers"`
		Wage     int64  `json:"wage"`
		Votes    int64  `json:"votes"`
		Shares   int64  `json:"shares"`
		Order    string `json:"order"`
		AutoBuy  bool   `json:"autoBuy"`
		BuyMax   int64  `json:"buyMax"`
		AutoSell bool   `json:"autoSell"`
		SellMin  int64  `json:"sellMin"`
		Improve  string `json:"improvement"`
		Contract string `json:"contract"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	d, err := a.firmsBackend.Run(r.Context(), token, model.FirmAct{
		Action: r.PathValue("action"), FirmID: req.Firm, Kind: req.Kind, Name: req.Name, Good: req.Good, Buy: req.Buy,
		Price: req.Price, Units: req.Units, Workers: req.Workers, Wage: req.Wage, Votes: req.Votes, Shares: req.Shares,
		OrderID: req.Order, AutoBuy: req.AutoBuy, BuyMax: req.BuyMax, AutoSell: req.AutoSell, SellMin: req.SellMin,
		Improvement: req.Improve, ContractID: req.Contract,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	firms, err := a.firmsResponse(r, d.Firms)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	type dealJSON struct {
		Good  string `json:"good"`
		Units int64  `json:"units"`
		Price int64  `json:"price"`
	}

	writeJSON(w, http.StatusOK, struct {
		Spent   int64      `json:"spent"`
		Got     int64      `json:"got"`
		Balance int64      `json:"balance"`
		Firm    string     `json:"firm"`
		Deals   []dealJSON `json:"deals"`
		Firms   firmsJSON  `json:"firms"`
	}{d.Spent, d.Got, d.Balance, d.FirmID, listJSON(d.Deals, func(deal model.FirmDeal) dealJSON { return dealJSON(deal) }), firms})
}

// firmSettings is "Бизнес" of the admin panel: the prices and how the
// enterprises do.
func (a *api) firmSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	s, stats, err := a.firmsBackend.Settings(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Settings firmSettingsJSON `json:"settings"`
		Firms    int64            `json:"firms"`
		Spent    int64            `json:"spent"`
		Paid     int64            `json:"paid"`
		Fees     int64            `json:"fees"`
	}{toFirmSettingsJSON(s), stats.Firms, stats.Spent, stats.Paid, stats.Fees})
}

func (a *api) setFirmSettings(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req firmSettingsJSON
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	settings := model.FirmSettings{
		Plans: listJSON(req.Plans, func(p firmPlanJSON) model.FirmPlan { return model.FirmPlan(p) }),
		Goods: listJSON(req.Goods, func(g goodPlanJSON) model.GoodPlan { return model.GoodPlan(g) }),
		Wage:  req.Wage, Fee: req.Fee, Floor: req.Floor, Ceiling: req.Ceiling, Retail: req.Retail,
		Storage: req.Storage, Capital: req.Capital,
		MaxFirms: req.MaxFirms, FoundingShares: req.FoundingShares, FloatShares: req.FloatShares, MaxShares: req.MaxShares,
	}

	if err = a.firmsBackend.Set(r.Context(), token, settings); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type userFirmJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Level    int    `json:"level"`
	Shares   int64  `json:"shares"`
	Total    int64  `json:"total"`
	Director bool   `json:"director"`
}

// getUserFirms are the enterprises a page holds shares of, public as the
// page.
func (a *api) getUserFirms(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	held, err := a.firmsBackend.UserFirms(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Firms []userFirmJSON `json:"firms"`
	}{listJSON(held, func(f model.UserFirm) userFirmJSON { return userFirmJSON(f) })})
}

func toFirmHourJSON(h model.FirmHour) firmHourJSON {
	return firmHourJSON{
		Hour: timeString(h.Hour), Made: h.Made, Bought: h.Bought, Sold: h.Sold, Income: h.Income,
		Purchases: h.Purchases, Wages: h.Wages, Upkeep: h.Upkeep, Fees: h.Fees,
	}
}
