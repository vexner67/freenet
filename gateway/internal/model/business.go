package model

import "time"

// BusinessPlan is a kind of business: the votes it costs and its next
// level costs times the level; customers an hour at the Fair price
// (hundredths), the cost of a unit of goods (hundredths), the storage, the
// customers a worker serves an hour and the workers a level employs; wages
// and rent an hour in hundredths; the condition lost an hour in hundredths
// of a percent and the votes a percent of repair costs; the till and the
// debt in votes times the level; an advertisement.
type BusinessPlan struct {
	Kind    string
	Price   int64
	Upgrade int64
	Levels  int
	Demand  int64
	Fair    int64
	Cost    int64
	Storage int64
	Serve   int64
	Staff   int
	Wage    int64
	Rent    int64
	Wear    int
	Repair  int64
	Till    int64
	Debt    int64
	AdPrice int64
	AdHours int
	AdBoost int
}

// BusinessSettings are the plans of all kinds in order and the tax percent
// of the till taken out.
type BusinessSettings struct {
	Plans []BusinessPlan
	Tax   int
}

// Business is a business of the caller now: price in hundredths,
// condition in hundredths of a percent, cash in hundredths (below zero — a
// debt), the advertisement's end (zero — none), an hour now and the votes
// it sells for.
type Business struct {
	Kind      string
	Level     int
	Stock     int64
	Price     int64
	Staff     int
	Condition int
	Cash      int64
	AdUntil   time.Time
	Bankrupt  bool
	Invested  int64
	Collected int64
	Customers float64
	Served    float64
	Income    float64
	Costs     float64
	Resale    int64
	InDebt    bool
	Perks     []BusinessPerk
	Storage   int64
}

// BusinessPerk is an improvement of a business: its tier, and the votes
// and the level its next tier costs and opens at (0 at the top tier).
type BusinessPerk struct {
	Perk      string
	Tier      int
	NextPrice int64
	NextLevel int
}

// Businesses are the businesses of the caller with the settings.
type Businesses struct {
	Settings   BusinessSettings
	Businesses []Business
}

// BusinessAct is an action: Amount is the units of a restock or the votes
// of a deposit, Price and Staff — what a manage sets, Perk — the
// improvement an improve buys the next tier of.
type BusinessAct struct {
	Kind   string
	Action string
	Amount int64
	Price  int64
	Staff  int
	Perk   string
}

// BusinessDone is what an action did: votes spent and got, the tax, the
// balance after them and the businesses.
type BusinessDone struct {
	Spent      int64
	Got        int64
	Tax        int64
	Balance    int64
	Businesses Businesses
}

// BusinessStats is how the businesses do: those open, votes spent on them
// and taken out.
type BusinessStats struct {
	Businesses int64
	Spent      int64
	Paid       int64
}
