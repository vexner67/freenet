package model

import "time"

// FirmPlan is an enterprise: the votes it costs to found and its next
// level costs times the level, the levels, the workers a level seats and
// the upkeep of a level an hour in hundredths of a vote.
type FirmPlan struct {
	Kind    string
	Price   int64
	Upgrade int64
	Levels  int
	Staff   int
	Upkeep  int64
}

// GoodPlan is a good: its base price in hundredths a unit and the units a
// day that halve what the state and the city pay.
type GoodPlan struct {
	Good string
	Base int64
	Norm int64
}

// FirmSettings are the enterprises and the goods in order, the standard
// wage in hundredths an hour, the fee of the exchange, what the state
// pays and asks and the city pays in percent of the base, the hours the
// storage holds, the percent of the price a new enterprise has, how many
// enterprises a page may direct and the shares of an enterprise: the
// founder's, those the exchange sells when it is founded and the most it
// may have.
type FirmSettings struct {
	Plans          []FirmPlan
	Goods          []GoodPlan
	Wage           int64
	Fee            int
	Floor          int
	Ceiling        int
	Retail         int
	Storage        int
	Capital        int
	MaxFirms       int
	FoundingShares int64
	FloatShares    int64
	MaxShares      int64
}

// FirmStats are how the enterprises do: how many there are, the votes
// people spent on them and got of them, the fees in hundredths.
type FirmStats struct {
	Firms int64
	Spent int64
	Paid  int64
	Fees  int64
}

// FirmInput is what a unit of the output takes.
type FirmInput struct {
	Good  string
	Units int64
}

// FirmSpec is what an enterprise does: rate units of output of a worker
// an hour, or of each good a shop sells.
type FirmSpec struct {
	Kind   string
	Tier   int
	Output string
	Inputs []FirmInput
	Sells  []string
	Rate   map[string]float64
}

// GoodQuote is a good now, in hundredths a unit.
type GoodQuote struct {
	Good      string
	StateBid  int64
	StateAsk  int64
	CityPrice int64
	BestBid   int64
	BestAsk   int64
	Last      int64
	Volume    int64
}

// FirmStock is a good an enterprise keeps and what it does with it by
// itself.
type FirmStock struct {
	Good     string
	Units    int64
	Capacity int64
	Incoming int64
	AutoBuy  bool
	BuyMax   int64
	AutoSell bool
	SellMin  int64
}

// FirmHour is an hour of an enterprise: units and hundredths, the
// workers hired and those who left for better pay.
type FirmHour struct {
	Hour      time.Time
	Made      int64
	Bought    int64
	Sold      int64
	Income    int64
	Purchases int64
	Wages     int64
	Upkeep    int64
	Fees      int64
	Hired     int64
	Left      int64
}

// FirmHolder is the shares a page holds.
type FirmHolder struct {
	UserID string
	Shares int64
}

// FirmImprovement is an improvement of an enterprise: its rank and the
// next rank's cost in hundredths (0 at the top).
type FirmImprovement struct {
	Kind string
	Rank int
	Cost int64
}

// Firm is an enterprise; money in hundredths of a vote, UpgradeCost in
// votes. Wear is the percent its machines are worn, IdleUntil the end of
// a strike, Price the last price of a share and Float the new shares up on
// the stock exchange.
type Firm struct {
	ID           string
	Kind         string
	Name         string
	DirectorID   string
	FoundedAt    time.Time
	Level        int
	Workers      int
	Wage         int64
	Cash         int64
	Escrow       int64
	Productivity float64
	Seats        int
	UpgradeCost  int64
	Upkeep       int64
	Profit       int64
	Shares       int64
	Worth        int64
	Stock        []FirmStock
	Hours        []FirmHour
	Holders      []FirmHolder
	Wear         float64
	RepairCost   int64
	IdleUntil    time.Time
	Improvements []FirmImprovement
	Price        int64
	Float        int64
	// Forecast is the hour going on as it ends if nothing changes and
	// ForecastCash the cash then; nil while ended hours wait to be counted.
	Forecast     *FirmHour
	ForecastCash int64
	// Wanted is the workers it seeks: Workers and the vacancies.
	Wanted int
	// Routes are the supplies it hands every hour to others of its
	// director.
	Routes []FirmRoute
}

// FirmRoute is a supply of an enterprise: Units of Good an hour to the
// enterprise ToFirmID, once From units are kept.
type FirmRoute struct {
	ID       string
	ToFirmID string
	Good     string
	From     int64
	Units    int64
}

// FirmLabour is the labour market of the city: its people, those working,
// the workers the enterprises seek and their average wage in hundredths.
type FirmLabour struct {
	People   int
	Employed int
	Jobs     int
	Wage     int64
}

// StockTender is an offer of UserID to buy Shares of FirmID from their
// holders at Price hundredths a share till Until.
type StockTender struct {
	ID       string
	FirmID   string
	UserID   string
	Price    int64
	Shares   int64
	PlacedAt time.Time
	Until    time.Time
}

// ExchangeOrder is an order of the exchange: Price in hundredths a unit.
type ExchangeOrder struct {
	ID       string
	FirmID   string
	Good     string
	Buy      bool
	Price    int64
	Units    int64
	PlacedAt time.Time
}

// FirmContract is an order of the city: Units of Good at Price
// hundredths a unit; FirmID — the enterprise that took it.
type FirmContract struct {
	ID        string
	Good      string
	Units     int64
	Price     int64
	Posted    time.Time
	Deadline  time.Time
	FirmID    string
	Delivered int64
}

// FirmEvent is what happened in the city: to the enterprise FirmID, or to
// all of Kinds (none — all).
type FirmEvent struct {
	ID     string
	Kind   string
	FirmID string
	Kinds  []string
	Start  time.Time
	End    time.Time
}

// Firms is the city of the enterprises.
type Firms struct {
	Settings  FirmSettings
	Catalog   []FirmSpec
	Quotes    []GoodQuote
	Firms     []Firm
	Orders    []ExchangeOrder
	Contracts []FirmContract
	News      []FirmEvent
	NextHour  time.Time
	Labour    FirmLabour
	Tenders   []StockTender
}

// FirmAct is an action with the enterprises.
type FirmAct struct {
	Action   string
	FirmID   string
	Kind     string
	Name     string
	Good     string
	Buy      bool
	Price    int64
	Units    int64
	Workers  int
	Wage     int64
	Votes    int64
	Shares   int64
	OrderID  string
	AutoBuy  bool
	BuyMax   int64
	AutoSell bool
	SellMin  int64
	// Improvement is what is improved, ContractID the order of the city.
	Improvement string
	ContractID  string
	// ToFirmID, From and RouteID are of the supplies: where to, from how
	// many units kept, and which one to stop.
	ToFirmID string
	From     int64
	RouteID  string
}

// FirmDeal is units that changed hands at a price in hundredths.
type FirmDeal struct {
	Good  string
	Units int64
	Price int64
}

// FirmDone is what an action did: the votes spent and got and the
// balance after (0 when none moved), the enterprise founded, the deals of
// an order and the city after.
type FirmDone struct {
	Spent   int64
	Got     int64
	Balance int64
	FirmID  string
	Deals   []FirmDeal
	Firms   Firms
}

// UserFirm is an enterprise as the profile of a shareholder shows it.
type UserFirm struct {
	ID       string
	Name     string
	Kind     string
	Level    int
	Shares   int64
	Total    int64
	Director bool
}

// StockCandle is an hour of a share's price in hundredths.
type StockCandle struct {
	Hour   time.Time
	Open   int64
	High   int64
	Low    int64
	Close  int64
	Volume int64
}

// StockCompany is an enterprise on the stock exchange, money in
// hundredths a share (Profit — of the last day).
type StockCompany struct {
	FirmID     string
	Name       string
	Kind       string
	Level      int
	DirectorID string
	Shares     int64
	Pool       int64
	Float      int64
	Book       int64
	Fair       int64
	Price      int64
	Bid        int64
	Ask        int64
	BestBid    int64
	BestAsk    int64
	DayAgo     int64
	Volume     int64
	Profit     int64
	Holders    int
	Candles    []StockCandle
	// BandLow and BandHigh are the prices orders of people may stand at.
	BandLow  int64
	BandHigh int64
}

// StockIndexPoint is the index at an hour, in hundredths of a point.
type StockIndexPoint struct {
	Hour  time.Time
	Value int64
}

// StockOrder is an order of shares at Price hundredths (0 — at any).
type StockOrder struct {
	ID       string
	FirmID   string
	Buy      bool
	Price    int64
	Shares   int64
	PlacedAt time.Time
}

// StockLevel is a price of the order book.
type StockLevel struct {
	Price  int64
	Shares int64
	Orders int
}

// StockHolding is shares of the caller; Free — not held by its orders.
type StockHolding struct {
	FirmID string
	Shares int64
	Free   int64
}

// Stocks is the stock exchange: Account is the caller's broker account in
// hundredths, Bids and Asks the order book of FirmID.
type Stocks struct {
	Companies []StockCompany
	Index     []StockIndexPoint
	News      []FirmEvent
	Fee       int
	Account   int64
	Holdings  []StockHolding
	Orders    []StockOrder
	FirmID    string
	Bids      []StockLevel
	Asks      []StockLevel
	NextHour  time.Time
	Tenders   []StockTender
}

// StockAct is an action on the stock exchange; OrderID is the order
// cancelled or the offer sold to or taken back.
type StockAct struct {
	Action  string
	FirmID  string
	Buy     bool
	Price   int64
	Shares  int64
	Votes   int64
	OrderID string
}

// StockDone is what an action did: the votes spent and got and the
// balance after (0 when none moved), what an order dealt at once — Shares
// for Money hundredths, and the exchange after.
type StockDone struct {
	Spent   int64
	Got     int64
	Balance int64
	Shares  int64
	Money   int64
	Stocks  Stocks
}
