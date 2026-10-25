package model

import "time"

// BusinessPlan is a kind of business: the votes it costs and its next
// level costs times the level; the demand in percent of the catalog's;
// seats a level adds; rent an hour in hundredths; the condition lost an
// hour in hundredths of a percent and the votes a percent of repair costs;
// the till and the debt in votes times the level; an advertisement.
type BusinessPlan struct {
	Kind    string
	Price   int64
	Upgrade int64
	Levels  int
	Demand  int64
	Staff   int
	Rent    int64
	Wear    int
	Repair  int64
	Till    int64
	Debt    int64
	AdPrice int64
	AdHours int
	AdBoost int
}

// BusinessSettings are the plans of all kinds in order, the tax percent of
// the till taken out, and the percents of the catalog wages and costs.
type BusinessSettings struct {
	Plans []BusinessPlan
	Tax   int
	Wages int
	Goods int
}

// BusinessCatalog is what a kind deals in: supplies, offers made of them
// and the professions that make them.
type BusinessCatalog struct {
	Kind        string
	Supplies    []BusinessSupplySpec
	Offers      []BusinessOfferSpec
	Professions []BusinessProfessionSpec
}

// BusinessSupplySpec is a supply: a unit's cost in hundredths, the percent
// spoiled a day and the storage a level adds.
type BusinessSupplySpec struct {
	Supply  string
	Cost    int64
	Spoil   int
	Storage int64
}

// BusinessOfferSpec is an offer: who makes it, of what, its fair price in
// hundredths and the customers an hour at it.
type BusinessOfferSpec struct {
	Offer      string
	Profession string
	Recipe     []BusinessIngredient
	Fair       int64
	Demand     float64
}

type BusinessIngredient struct {
	Supply string
	Units  int64
}

// BusinessProfessionSpec is a profession: the wage an hour in hundredths
// and the customers a worker of it serves an hour.
type BusinessProfessionSpec struct {
	Profession string
	Wage       int64
	Serve      float64
}

// BusinessSupplier is a supplier: the percent of the cost it asks, the
// hours a delivery takes and the percent it may come short by.
type BusinessSupplier struct {
	Supplier string
	Percent  int64
	Hours    float64
	Short    int
}

// Business is a business of the caller now: condition in hundredths of a
// percent, cash in hundredths (below zero — a debt), what it has, who works
// there, what is on the way and what happened.
type Business struct {
	Kind            string
	Level           int
	Condition       int
	Cash            int64
	AdUntil         time.Time
	Bankrupt        bool
	Invested        int64
	Collected       int64
	Resale          int64
	InDebt          bool
	Supplies        []BusinessStock
	Offers          []BusinessOffer
	Workers         []BusinessWorker
	Candidates      []BusinessCandidate
	CandidatesUntil time.Time
	Orders          []BusinessOrder
	Contracts       []BusinessContract
	Event           *BusinessEvent
	Notes           []BusinessNote
	Hours           []BusinessHour
	Seats           int
	ReviewUntil     time.Time
	Tax             int
	Wages           float64
	Rent            float64
	Till            int64
	DebtLimit       int64
	Load            map[string]float64
	Broken          string
	BrokenUntil     time.Time
	UpgradeCost     int64
	RepairCost      int64
}

type BusinessStock struct {
	Supply    string
	Units     float64
	OnWay     int64
	Storage   int64
	UnitCosts map[string]float64
	Use       float64
}

type BusinessOffer struct {
	Offer    string
	Price    int64
	Demand   float64
	Served   float64
	Capacity float64
	Broken   bool
}

type BusinessWorker struct {
	ID         int
	Name       string
	Female     bool
	Profession string
	Skill      int
	XP         float64
	NextXP     float64
	Wage       int64
	Trait      string
	Mood       float64
	Rate       float64
	Bonus      int64
}

type BusinessCandidate struct {
	ID     int64
	Worker BusinessWorker
}

type BusinessOrder struct {
	ID       int
	Supply   string
	Supplier string
	Units    int64
	Arrives  time.Time
	FromTill bool
}

type BusinessContract struct {
	Supply   string
	Supplier string
	Below    int64
	Units    int64
	Stalled  bool
}

// BusinessEvent is what waits for an answer until Until.
type BusinessEvent struct {
	Kind    string
	Until   time.Time
	Offer   string
	Worker  int
	Name    string
	Female  bool
	Votes   int64
	Units   int64
	Pay     int64
	Options []string
}

type BusinessNote struct {
	At      time.Time
	Kind    string
	Subject string
	Name    string
	Female  bool
	Amount  float64
	Money   float64
}

// BusinessHour is what an hour brought, in hundredths of a vote.
type BusinessHour struct {
	Hour     time.Time
	Sold     map[string]float64
	Income   float64
	Wages    float64
	Rent     float64
	Supplies float64
	Losses   float64
	Spoiled  float64
	Unserved float64
}

// Businesses are the businesses of the caller with the settings, the
// catalog of each kind and the suppliers.
type Businesses struct {
	Settings   BusinessSettings
	Businesses []Business
	Catalog    []BusinessCatalog
	Suppliers  []BusinessSupplier
}

// BusinessAct is an action with a business: Amount is the units of an
// order or a contract or the votes of a deposit, Price — of an offer in
// hundredths, Below — what a contract keeps the supply over, Option — the
// answer to the event.
type BusinessAct struct {
	Kind      string
	Action    string
	Amount    int64
	Price     int64
	Offer     string
	Supply    string
	Supplier  string
	Below     int64
	Worker    int
	Candidate int64
	Option    string
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
