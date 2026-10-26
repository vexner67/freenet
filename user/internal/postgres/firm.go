package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// FirmRepository keeps the economy of the enterprises: the one row of
// firm_city with the rules and the whole city.
type FirmRepository struct {
	pool *pgxpool.Pool
}

func NewFirmRepository(pool *pgxpool.Pool) *FirmRepository {
	return &FirmRepository{pool: pool}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *FirmRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// The rules and the city as the columns keep them. The parts are the
// domain structs with JSON names: a struct converts to another with the
// same fields whatever their tags.
type storedFirmPlan struct {
	Kind    user.FirmKind `json:"kind"`
	Price   int64         `json:"price"`
	Upgrade int64         `json:"upgrade"`
	Levels  int           `json:"levels"`
	Staff   int           `json:"staff"`
	Upkeep  int64         `json:"upkeep"`
}

type storedGoodPlan struct {
	Good user.Good `json:"good"`
	Base int64     `json:"base"`
	Norm int64     `json:"norm"`
}

type storedFirmRules struct {
	Plans   []storedFirmPlan `json:"plans"`
	Goods   []storedGoodPlan `json:"goods"`
	Wage    int64            `json:"wage"`
	Fee     int              `json:"fee"`
	Floor   int              `json:"floor"`
	Ceiling int              `json:"ceiling"`
	Retail  int              `json:"retail"`
	Storage int              `json:"storage"`
	Capital int              `json:"capital"`
}

type storedAuto struct {
	Buy     bool  `json:"buy"`
	BuyMax  int64 `json:"buyMax"`
	Sell    bool  `json:"sell"`
	SellMin int64 `json:"sellMin"`
}

type storedFirmHour struct {
	Hour      time.Time `json:"hour"`
	Made      int64     `json:"made"`
	Bought    int64     `json:"bought"`
	Sold      int64     `json:"sold"`
	Income    int64     `json:"income"`
	Purchases int64     `json:"purchases"`
	Wages     int64     `json:"wages"`
	Upkeep    int64     `json:"upkeep"`
	Fees      int64     `json:"fees"`
}

type storedFirm struct {
	ID         uuid.UUID                `json:"id"`
	Kind       user.FirmKind            `json:"kind"`
	Name       user.FirmName            `json:"name"`
	DirectorID uuid.UUID                `json:"directorId"`
	FoundedAt  time.Time                `json:"foundedAt"`
	Level      int                      `json:"level"`
	Workers    int                      `json:"workers"`
	Wage       int64                    `json:"wage"`
	Cash       int64                    `json:"cash"`
	Stock      map[user.Good]int64      `json:"stock"`
	Carry      map[user.Good]float64    `json:"carry"`
	Auto       map[user.Good]storedAuto `json:"auto"`
	Hours      []storedFirmHour         `json:"hours"`
	SharePrice int64                    `json:"sharePrice"`
}

type storedHolding struct {
	FirmID uuid.UUID `json:"firmId"`
	UserID uuid.UUID `json:"userId"`
	Shares int64     `json:"shares"`
}

type storedExchangeOrder struct {
	ID       uuid.UUID `json:"id"`
	FirmID   uuid.UUID `json:"firmId"`
	Good     user.Good `json:"good"`
	Buy      bool      `json:"buy"`
	Price    int64     `json:"price"`
	Units    int64     `json:"units"`
	PlacedAt time.Time `json:"placedAt"`
}

type storedShareOffer struct {
	ID       uuid.UUID `json:"id"`
	FirmID   uuid.UUID `json:"firmId"`
	SellerID uuid.UUID `json:"sellerId"`
	Shares   int64     `json:"shares"`
	Price    int64     `json:"price"`
	PlacedAt time.Time `json:"placedAt"`
}

type storedMarketHour struct {
	Hour        time.Time `json:"hour"`
	Good        user.Good `json:"good"`
	StateSold   int64     `json:"stateSold"`
	StateBought int64     `json:"stateBought"`
	City        int64     `json:"city"`
	Traded      int64     `json:"traded"`
	Turnover    int64     `json:"turnover"`
}

type storedCityStats struct {
	Spent int64 `json:"spent"`
	Paid  int64 `json:"paid"`
	Fees  int64 `json:"fees"`
}

type storedCity struct {
	Hour     time.Time             `json:"hour"`
	Firms    []storedFirm          `json:"firms"`
	Holdings []storedHolding       `json:"holdings"`
	Orders   []storedExchangeOrder `json:"orders"`
	Offers   []storedShareOffer    `json:"offers"`
	Market   []storedMarketHour    `json:"market"`
	Last     map[user.Good]int64   `json:"last"`
	Stats    storedCityStats       `json:"stats"`
}

func convert[From, To any](from []From, to func(From) To) []To {
	out := make([]To, len(from))
	for i, f := range from {
		out[i] = to(f)
	}

	return out
}

func storeFirm(f user.FirmState) storedFirm {
	auto := map[user.Good]storedAuto{}
	for g, a := range f.Auto {
		auto[g] = storedAuto(a)
	}

	return storedFirm{
		ID: f.ID, Kind: f.Kind, Name: f.Name, DirectorID: f.DirectorID, FoundedAt: f.FoundedAt, Level: f.Level,
		Workers: f.Workers, Wage: f.Wage, Cash: f.Cash, Stock: f.Stock, Carry: f.Carry, Auto: auto,
		Hours:      convert(f.Hours, func(h user.FirmHour) storedFirmHour { return storedFirmHour(h) }),
		SharePrice: f.SharePrice,
	}
}

func restoreFirm(f storedFirm) user.FirmState {
	auto := map[user.Good]user.Auto{}
	for g, a := range f.Auto {
		auto[g] = user.Auto(a)
	}

	return user.FirmState{
		ID: f.ID, Kind: f.Kind, Name: f.Name, DirectorID: f.DirectorID, FoundedAt: f.FoundedAt, Level: f.Level,
		Workers: f.Workers, Wage: f.Wage, Cash: f.Cash, Stock: f.Stock, Carry: f.Carry, Auto: auto,
		Hours:      convert(f.Hours, func(h storedFirmHour) user.FirmHour { return user.FirmHour(h) }),
		SharePrice: f.SharePrice,
	}
}

func storeCity(c *user.City) ([]byte, error) {
	s := c.State()
	data, err := json.Marshal(storedCity{
		Hour:     s.Hour,
		Firms:    convert(s.Firms, storeFirm),
		Holdings: convert(s.Holdings, func(h user.Holding) storedHolding { return storedHolding(h) }),
		Orders:   convert(s.Orders, func(o user.ExchangeOrder) storedExchangeOrder { return storedExchangeOrder(o) }),
		Offers:   convert(s.Offers, func(o user.ShareOffer) storedShareOffer { return storedShareOffer(o) }),
		Market:   convert(s.Market, func(m user.MarketHour) storedMarketHour { return storedMarketHour(m) }),
		Last:     s.Last,
		Stats:    storedCityStats(s.Stats),
	})

	return data, errs.Wrap("json.Marshal", err)
}

func restoreCity(s storedCity) *user.City {
	return user.RestoreCity(user.CityState{
		Hour:     s.Hour,
		Firms:    convert(s.Firms, restoreFirm),
		Holdings: convert(s.Holdings, func(h storedHolding) user.Holding { return user.Holding(h) }),
		Orders:   convert(s.Orders, func(o storedExchangeOrder) user.ExchangeOrder { return user.ExchangeOrder(o) }),
		Offers:   convert(s.Offers, func(o storedShareOffer) user.ShareOffer { return user.ShareOffer(o) }),
		Market:   convert(s.Market, func(m storedMarketHour) user.MarketHour { return user.MarketHour(m) }),
		Last:     s.Last,
		Stats:    user.CityStats(s.Stats),
	})
}

const getFirmRulesQuery = `
	SELECT settings
	FROM firm_city
`

func (r *FirmRepository) Rules(ctx context.Context) (user.FirmRules, error) {
	var stored *storedFirmRules

	if err := r.conn(ctx).QueryRow(ctx, getFirmRulesQuery).Scan(&stored); err != nil {
		return user.FirmRules{}, errs.Wrap("r.conn.QueryRow", err)
	}
	if stored == nil {
		return user.DefaultFirmRules(), nil
	}

	rules, err := user.NewFirmRules(user.FirmRules{
		Plans:   convert(stored.Plans, func(p storedFirmPlan) user.FirmPlan { return user.FirmPlan(p) }),
		Goods:   convert(stored.Goods, func(g storedGoodPlan) user.GoodPlan { return user.GoodPlan(g) }),
		Wage:    stored.Wage,
		Fee:     stored.Fee,
		Floor:   stored.Floor,
		Ceiling: stored.Ceiling,
		Retail:  stored.Retail,
		Storage: stored.Storage,
		Capital: stored.Capital,
	})

	return rules, errs.Wrap("user.NewFirmRules", err)
}

const setFirmRulesQuery = `
	UPDATE firm_city
	SET settings = $1
`

func (r *FirmRepository) SetRules(ctx context.Context, rules user.FirmRules) error {
	data, err := json.Marshal(storedFirmRules{
		Plans:   convert(rules.Plans, func(p user.FirmPlan) storedFirmPlan { return storedFirmPlan(p) }),
		Goods:   convert(rules.Goods, func(g user.GoodPlan) storedGoodPlan { return storedGoodPlan(g) }),
		Wage:    rules.Wage,
		Fee:     rules.Fee,
		Floor:   rules.Floor,
		Ceiling: rules.Ceiling,
		Retail:  rules.Retail,
		Storage: rules.Storage,
		Capital: rules.Capital,
	})
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.conn(ctx).Exec(ctx, setFirmRulesQuery, data)

	return errs.Wrap("r.conn.Exec", err)
}

const (
	getCityQuery = `
	SELECT state
	FROM firm_city
`
	lockCityQuery = getCityQuery + `FOR UPDATE`
)

func (r *FirmRepository) City(ctx context.Context, lock bool) (*user.City, error) {
	query := getCityQuery
	if lock {
		query = lockCityQuery
	}

	var stored *storedCity
	if err := r.conn(ctx).QueryRow(ctx, query).Scan(&stored); err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}
	if stored == nil {
		return user.NewCity(time.Now()), nil
	}

	return restoreCity(*stored), nil
}

const saveCityQuery = `
	UPDATE firm_city
	SET state = $1
`

func (r *FirmRepository) SaveCity(ctx context.Context, c *user.City) error {
	data, err := storeCity(c)
	if err != nil {
		return errs.Wrap("storeCity", err)
	}

	_, err = r.conn(ctx).Exec(ctx, saveCityQuery, data)

	return errs.Wrap("r.conn.Exec", err)
}
