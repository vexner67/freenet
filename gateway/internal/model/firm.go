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
// storage holds and the percent of the price a new enterprise has.
type FirmSettings struct {
	Plans   []FirmPlan
	Goods   []GoodPlan
	Wage    int64
	Fee     int
	Floor   int
	Ceiling int
	Retail  int
	Storage int
	Capital int
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

// FirmHour is an hour of an enterprise: units and hundredths.
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
}

// FirmHolder is the shares a page holds.
type FirmHolder struct {
	UserID string
	Shares int64
}

// Firm is an enterprise; money in hundredths of a vote, UpgradeCost and
// SharePrice in votes.
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
	SharePrice   int64
	Worth        int64
	Stock        []FirmStock
	Hours        []FirmHour
	Holders      []FirmHolder
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

// ShareOffer is shares for sale at Price votes each; no SellerID — new
// shares of the enterprise.
type ShareOffer struct {
	ID       string
	FirmID   string
	SellerID string
	Shares   int64
	Price    int64
	PlacedAt time.Time
}

// Firms is the city of the enterprises.
type Firms struct {
	Settings FirmSettings
	Catalog  []FirmSpec
	Quotes   []GoodQuote
	Firms    []Firm
	Orders   []ExchangeOrder
	Offers   []ShareOffer
	NextHour time.Time
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
	OfferID  string
	AutoBuy  bool
	BuyMax   int64
	AutoSell bool
	SellMin  int64
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
