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
	// Rules stored before the administrators could set these have none of
	// them: they get the default ones.
	MaxFirms  *int   `json:"maxFirms,omitempty"`
	Founding  *int64 `json:"founding,omitempty"`
	Float     *int64 `json:"float,omitempty"`
	MaxShares *int64 `json:"maxShares,omitempty"`
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
	Hired     int64     `json:"hired,omitempty"`
	Left      int64     `json:"left,omitempty"`
}

type storedFirm struct {
	ID         uuid.UUID                `json:"id"`
	Kind       user.FirmKind            `json:"kind"`
	Name       user.FirmName            `json:"name"`
	DirectorID uuid.UUID                `json:"directorId"`
	FoundedAt  time.Time                `json:"foundedAt"`
	Level      int                      `json:"level"`
	Workers    int                      `json:"workers"`
	Wanted     int                      `json:"wanted,omitempty"`
	Wage       int64                    `json:"wage"`
	Cash       int64                    `json:"cash"`
	Stock      map[user.Good]int64      `json:"stock"`
	Carry      map[user.Good]float64    `json:"carry"`
	Auto       map[user.Good]storedAuto `json:"auto"`
	Hours      []storedFirmHour         `json:"hours"`
	Wear       float64                  `json:"wear"`
	Ranks      map[user.Improvement]int `json:"ranks"`
	Improved   int64                    `json:"improved"`
	IdleUntil  time.Time                `json:"idleUntil"`
	Mood       float64                  `json:"mood"`
	Price      int64                    `json:"price"`
	Float      int64                    `json:"float"`
	Candles    []storedCandle           `json:"candles"`
}

type storedCandle struct {
	Hour   time.Time `json:"hour"`
	Open   int64     `json:"open"`
	High   int64     `json:"high"`
	Low    int64     `json:"low"`
	Close  int64     `json:"close"`
	Volume int64     `json:"volume"`
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

type storedAccount struct {
	UserID uuid.UUID `json:"userId"`
	Cash   int64     `json:"cash"`
}

type storedStockOrder struct {
	ID       uuid.UUID `json:"id"`
	FirmID   uuid.UUID `json:"firmId"`
	UserID   uuid.UUID `json:"userId"`
	Buy      bool      `json:"buy"`
	Price    int64     `json:"price"`
	Shares   int64     `json:"shares"`
	PlacedAt time.Time `json:"placedAt"`
}

type storedStockHour struct {
	Hour   time.Time `json:"hour"`
	FirmID uuid.UUID `json:"firmId"`
	Sold   int64     `json:"sold"`
	Bought int64     `json:"bought"`
}

type storedIndexPoint struct {
	Hour  time.Time `json:"hour"`
	Value int64     `json:"value"`
}

type storedEvent struct {
	ID     uuid.UUID      `json:"id"`
	Kind   user.EventKind `json:"kind"`
	FirmID uuid.UUID      `json:"firmId"`
	Start  time.Time      `json:"start"`
	End    time.Time      `json:"end"`
}

type storedContract struct {
	ID        uuid.UUID `json:"id"`
	Good      user.Good `json:"good"`
	Units     int64     `json:"units"`
	Price     int64     `json:"price"`
	Posted    time.Time `json:"posted"`
	Deadline  time.Time `json:"deadline"`
	FirmID    uuid.UUID `json:"firmId"`
	Delivered int64     `json:"delivered"`
}

type storedTender struct {
	ID       uuid.UUID `json:"id"`
	FirmID   uuid.UUID `json:"firmId"`
	UserID   uuid.UUID `json:"userId"`
	Price    int64     `json:"price"`
	Shares   int64     `json:"shares"`
	PlacedAt time.Time `json:"placedAt"`
	Until    time.Time `json:"until"`
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

// storedCity keeps Offers, the offers of shares before the stock
// exchange, only to read them: the city turns them into its orders. A city
// kept before the labour market has no People: it gets those it employs.
type storedCity struct {
	Hour        time.Time             `json:"hour"`
	People      int                   `json:"people,omitempty"`
	Firms       []storedFirm          `json:"firms"`
	Holdings    []storedHolding       `json:"holdings"`
	Orders      []storedExchangeOrder `json:"orders"`
	Offers      []storedShareOffer    `json:"offers,omitempty"`
	Market      []storedMarketHour    `json:"market"`
	Last        map[user.Good]int64   `json:"last"`
	Stats       storedCityStats       `json:"stats"`
	Accounts    []storedAccount       `json:"accounts"`
	StockOrders []storedStockOrder    `json:"stockOrders"`
	StockMarket []storedStockHour     `json:"stockMarket"`
	Index       []storedIndexPoint    `json:"index"`
	Events      []storedEvent         `json:"events"`
	Contracts   []storedContract      `json:"contracts"`
	Tenders     []storedTender        `json:"tenders,omitempty"`
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
		Workers: f.Workers, Wanted: f.Wanted, Wage: f.Wage, Cash: f.Cash, Stock: f.Stock, Carry: f.Carry, Auto: auto,
		Hours: convert(f.Hours, func(h user.FirmHour) storedFirmHour { return storedFirmHour(h) }),
		Wear:  f.Wear, Ranks: f.Ranks, Improved: f.Improved, IdleUntil: f.IdleUntil, Mood: f.Mood, Price: f.Price,
		Float: f.Float, Candles: convert(f.Candles, func(c user.Candle) storedCandle { return storedCandle(c) }),
	}
}

func restoreFirm(f storedFirm) user.FirmState {
	auto := map[user.Good]user.Auto{}
	for g, a := range f.Auto {
		auto[g] = user.Auto(a)
	}

	return user.FirmState{
		ID: f.ID, Kind: f.Kind, Name: f.Name, DirectorID: f.DirectorID, FoundedAt: f.FoundedAt, Level: f.Level,
		Workers: f.Workers, Wanted: f.Wanted, Wage: f.Wage, Cash: f.Cash, Stock: f.Stock, Carry: f.Carry, Auto: auto,
		Hours: convert(f.Hours, func(h storedFirmHour) user.FirmHour { return user.FirmHour(h) }),
		Wear:  f.Wear, Ranks: f.Ranks, Improved: f.Improved, IdleUntil: f.IdleUntil, Mood: f.Mood, Price: f.Price,
		Float: f.Float, Candles: convert(f.Candles, func(c storedCandle) user.Candle { return user.Candle(c) }),
	}
}

func storeCity(c *user.City) ([]byte, error) {
	s := c.State()
	data, err := json.Marshal(storedCity{
		Hour:        s.Hour,
		People:      s.People,
		Firms:       convert(s.Firms, storeFirm),
		Holdings:    convert(s.Holdings, func(h user.Holding) storedHolding { return storedHolding(h) }),
		Orders:      convert(s.Orders, func(o user.ExchangeOrder) storedExchangeOrder { return storedExchangeOrder(o) }),
		Market:      convert(s.Market, func(m user.MarketHour) storedMarketHour { return storedMarketHour(m) }),
		Last:        s.Last,
		Stats:       storedCityStats(s.Stats),
		Accounts:    convert(s.Accounts, func(a user.Account) storedAccount { return storedAccount(a) }),
		StockOrders: convert(s.StockOrders, func(o user.StockOrder) storedStockOrder { return storedStockOrder(o) }),
		StockMarket: convert(s.StockMarket, func(h user.StockHour) storedStockHour { return storedStockHour(h) }),
		Index:       convert(s.Index, func(p user.IndexPoint) storedIndexPoint { return storedIndexPoint(p) }),
		Events:      convert(s.Events, func(e user.Event) storedEvent { return storedEvent(e) }),
		Contracts:   convert(s.Contracts, func(k user.Contract) storedContract { return storedContract(k) }),
		Tenders:     convert(s.Tenders, func(t user.Tender) storedTender { return storedTender(t) }),
	})

	return data, errs.Wrap("json.Marshal", err)
}

func restoreCity(s storedCity) *user.City {
	return user.RestoreCity(user.CityState{
		Hour:        s.Hour,
		People:      s.People,
		Firms:       convert(s.Firms, restoreFirm),
		Holdings:    convert(s.Holdings, func(h storedHolding) user.Holding { return user.Holding(h) }),
		Orders:      convert(s.Orders, func(o storedExchangeOrder) user.ExchangeOrder { return user.ExchangeOrder(o) }),
		Offers:      convert(s.Offers, func(o storedShareOffer) user.ShareOffer { return user.ShareOffer(o) }),
		Market:      convert(s.Market, func(m storedMarketHour) user.MarketHour { return user.MarketHour(m) }),
		Last:        s.Last,
		Stats:       user.CityStats(s.Stats),
		Accounts:    convert(s.Accounts, func(a storedAccount) user.Account { return user.Account(a) }),
		StockOrders: convert(s.StockOrders, func(o storedStockOrder) user.StockOrder { return user.StockOrder(o) }),
		StockMarket: convert(s.StockMarket, func(h storedStockHour) user.StockHour { return user.StockHour(h) }),
		Index:       convert(s.Index, func(p storedIndexPoint) user.IndexPoint { return user.IndexPoint(p) }),
		Events:      convert(s.Events, func(e storedEvent) user.Event { return user.Event(e) }),
		Contracts:   convert(s.Contracts, func(k storedContract) user.Contract { return user.Contract(k) }),
		Tenders:     convert(s.Tenders, func(t storedTender) user.Tender { return user.Tender(t) }),
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

	defaults := user.DefaultFirmRules()
	rules, err := user.NewFirmRules(user.FirmRules{
		Plans:     convert(stored.Plans, func(p storedFirmPlan) user.FirmPlan { return user.FirmPlan(p) }),
		Goods:     convert(stored.Goods, func(g storedGoodPlan) user.GoodPlan { return user.GoodPlan(g) }),
		Wage:      stored.Wage,
		Fee:       stored.Fee,
		Floor:     stored.Floor,
		Ceiling:   stored.Ceiling,
		Retail:    stored.Retail,
		Storage:   stored.Storage,
		Capital:   stored.Capital,
		MaxFirms:  orDefault(stored.MaxFirms, defaults.MaxFirms),
		Founding:  orDefault(stored.Founding, defaults.Founding),
		Float:     orDefault(stored.Float, defaults.Float),
		MaxShares: orDefault(stored.MaxShares, defaults.MaxShares),
	})

	return rules, errs.Wrap("user.NewFirmRules", err)
}

const setFirmRulesQuery = `
	UPDATE firm_city
	SET settings = $1
`

func (r *FirmRepository) SetRules(ctx context.Context, rules user.FirmRules) error {
	data, err := json.Marshal(storedFirmRules{
		Plans:     convert(rules.Plans, func(p user.FirmPlan) storedFirmPlan { return storedFirmPlan(p) }),
		Goods:     convert(rules.Goods, func(g user.GoodPlan) storedGoodPlan { return storedGoodPlan(g) }),
		Wage:      rules.Wage,
		Fee:       rules.Fee,
		Floor:     rules.Floor,
		Ceiling:   rules.Ceiling,
		Retail:    rules.Retail,
		Storage:   rules.Storage,
		Capital:   rules.Capital,
		MaxFirms:  &rules.MaxFirms,
		Founding:  &rules.Founding,
		Float:     &rules.Float,
		MaxShares: &rules.MaxShares,
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

// orDefault is the stored value, or fallback when the rules were stored
// without it.
func orDefault[T any](stored *T, fallback T) T {
	if stored == nil {
		return fallback
	}

	return *stored
}
