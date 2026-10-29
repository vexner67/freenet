package service

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// FirmRepository keeps the enterprises of the site: the city and its
// rules.
type FirmRepository interface {
	// Rules are user.DefaultFirmRules until some are set.
	Rules(ctx context.Context) (user.FirmRules, error)
	SetRules(ctx context.Context, rules user.FirmRules) error
	// City is the city of the enterprises; locked till the end of the
	// transaction with lock, so that one change goes at a time.
	City(ctx context.Context, lock bool) (*user.City, error)
	SaveCity(ctx context.Context, c *user.City) error
}

// FirmsView is the city as people see it at a time.
type FirmsView struct {
	Rules user.FirmRules
	City  *user.City
	At    time.Time
	// Forecast is how the hour going on ends for each enterprise; none
	// while the city has ended hours yet to count.
	Forecast map[uuid.UUID]user.FirmForecast
}

func newFirmsView(rules user.FirmRules, city *user.City, now time.Time) FirmsView {
	v := FirmsView{Rules: rules, City: city, At: now}
	if city.Hour().Add(time.Hour).After(now) {
		v.Forecast = city.Forecast(rules)
	}

	return v
}

// FirmAct is an action with the enterprises: FirmID is the enterprise
// (Kind and Name found one), Order an order of the exchange of goods
// (Good, Buy, Price, Units also the good and units of a deal with the
// state or of a delivery), Auto what it does with Good by itself; Workers
// and Wage the staff, Votes a deposit, dividends or money of the broker
// account, Shares the shares issued, Stock an order of shares,
// Improvement what is improved, ContractID the order of the city taken or
// delivered, OrderID what is cancelled or the offer sold to.
type FirmAct struct {
	Action      user.FirmAction
	FirmID      uuid.UUID
	Kind        user.FirmKind
	Name        user.FirmName
	Order       user.ExchangeOrder
	Auto        user.Auto
	Workers     int
	Wage        int64
	Votes       int64
	Shares      int64
	Stock       user.StockOrder
	Improvement user.Improvement
	ContractID  uuid.UUID
	OrderID     uuid.UUID
	Route       user.SupplyRoute
	RouteID     uuid.UUID
}

// FirmDone is what an action did: the votes the caller spent and got and
// the balance after them (0 when no votes of the caller moved), the
// enterprise founded, the deals of the exchange, what an order of shares
// dealt at once, and the city after it.
type FirmDone struct {
	Spent   int64
	Got     int64
	Balance int64
	FirmID  uuid.UUID
	Deals   []user.Deal
	Fill    user.StockFill
	View    FirmsView
}

// UserFirm is an enterprise a page holds shares of.
type UserFirm struct {
	Firm     *user.Firm
	Shares   int64
	Total    int64
	Director bool
}

// FirmSettings are the rules and how the enterprises do: for the
// administrators.
type FirmSettings struct {
	Rules user.FirmRules
	Stats user.CityStats
	Firms int
}

// FirmService is "Бизнес" and "Акции": the economy of the enterprises.
// People found enterprises for votes into the treasury, the enterprises
// hire the people of the city, make and trade goods hour by hour with
// each other on the exchange and with the state and the city, take its
// orders, and pay their shareholders dividends; shares change hands on
// the stock exchange, paid of the broker accounts, and offers buy out the
// shareholders.
type FirmService struct {
	userRepo UserRepository
	firms    FirmRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
}

func NewFirmService(users UserRepository, firms FirmRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *FirmService {
	return &FirmService{userRepo: users, firms: firms, wallets: wallets, tx: tx, admins: admins}
}

// View is the city at now: as last counted, the hours ended since are
// counted by Tick.
func (s *FirmService) View(ctx context.Context, now time.Time) (FirmsView, error) {
	rules, err := s.firms.Rules(ctx)
	if err != nil {
		return FirmsView{}, errs.Wrap("s.firms.Rules", err)
	}

	city, err := s.firms.City(ctx, false)
	if err != nil {
		return FirmsView{}, errs.Wrap("s.firms.City", err)
	}

	return newFirmsView(rules, city, now), nil
}

// UserFirms are the enterprises the page nickname holds shares of, those
// it directs first; user.ErrNotFound for a page deleted or banned at now.
func (s *FirmService) UserFirms(ctx context.Context, nickname user.Nickname, now time.Time) ([]UserFirm, error) {
	owner, err := s.userRepo.GetByNickname(ctx, nickname)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByNickname", err)
	}

	if owner.IsDeactivated() || owner.IsBannedAt(now) {
		return nil, user.ErrNotFound
	}

	city, err := s.firms.City(ctx, false)
	if err != nil {
		return nil, errs.Wrap("s.firms.City", err)
	}

	var out []UserFirm
	for _, h := range city.HoldingsOf(owner.ID()) {
		f, err := city.Firm(h.FirmID)
		if err != nil {
			continue
		}

		out = append(out, UserFirm{Firm: f, Shares: h.Shares, Total: city.Shares(f.ID()), Director: f.DirectorID() == owner.ID()})
	}

	slices.SortStableFunc(out, func(a, b UserFirm) int {
		switch {
		case a.Director == b.Director:
			return 0
		case a.Director:
			return -1
		default:
			return 1
		}
	})

	return out, nil
}

// Tick counts the hours of the city ended by now; how many it counted.
func (s *FirmService) Tick(ctx context.Context, now time.Time) (int, error) {
	counted := 0

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		rules, err := s.firms.Rules(ctx)
		if err != nil {
			return errs.Wrap("s.firms.Rules", err)
		}

		city, err := s.firms.City(ctx, true)
		if err != nil {
			return errs.Wrap("s.firms.City", err)
		}

		if !city.Hour().Add(time.Hour).After(now) {
			if err = s.prune(ctx, city); err != nil {
				return errs.Wrap("s.prune", err)
			}

			counted = city.Tick(rules, now)
		}
		if counted == 0 {
			return nil
		}

		return errs.Wrap("s.firms.SaveCity", s.firms.SaveCity(ctx, city))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return counted, nil
}

// prune drops the shareholders whose pages are gone for good.
func (s *FirmService) prune(ctx context.Context, city *user.City) error {
	holders := city.Holders()
	if len(holders) == 0 {
		return nil
	}

	found, err := s.userRepo.GetByIDs(ctx, holders)
	if err != nil {
		return errs.Wrap("s.userRepo.GetByIDs", err)
	}

	gone := slices.DeleteFunc(holders, func(id uuid.UUID) bool {
		return slices.ContainsFunc(found, func(u *user.User) bool { return u.ID() == id })
	})
	city.DropHolders(gone)

	return nil
}

func firmNote(action user.FirmAction) user.VoteEntry {
	return user.Entry(user.VoteBusiness, uuid.Nil, string(action))
}

// Act does act for userID at now, the hours ended counted first. Taking
// votes out — dividends, closing, withdrawing from the broker account — is
// for the pages that take the daily bonus (user.ErrFirmNotYet).
func (s *FirmService) Act(ctx context.Context, userID uuid.UUID, act FirmAct, now time.Time) (FirmDone, error) {
	if act.Action.PaysOut() {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return FirmDone{}, errs.Wrap("s.userRepo.GetByID", err)
		}
		if err = u.CheckFirmsAt(now); err != nil {
			return FirmDone{}, errs.Wrap("u.CheckFirmsAt", err)
		}
	}

	var done FirmDone

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		rules, err := s.firms.Rules(ctx)
		if err != nil {
			return errs.Wrap("s.firms.Rules", err)
		}

		city, err := s.firms.City(ctx, true)
		if err != nil {
			return errs.Wrap("s.firms.City", err)
		}

		if err = s.prune(ctx, city); err != nil {
			return errs.Wrap("s.prune", err)
		}
		city.Tick(rules, now)

		run := firmRun{s: s, rules: rules, city: city, userID: userID, act: act, now: now, done: &done}
		if err = run.do(ctx); err != nil {
			return errs.Wrap("run.do", err)
		}

		done.View = newFirmsView(rules, city, now)

		return errs.Wrap("s.firms.SaveCity", s.firms.SaveCity(ctx, city))
	})
	if err != nil {
		return FirmDone{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return done, nil
}

// firmRun is an action being done in its transaction.
type firmRun struct {
	s      *FirmService
	rules  user.FirmRules
	city   *user.City
	userID uuid.UUID
	act    FirmAct
	now    time.Time
	done   *FirmDone
}

func (r firmRun) do(ctx context.Context) error {
	a, c := r.act, r.city

	switch a.Action {
	case user.FirmFound:
		return r.found(ctx)
	case user.FirmRename:
		return errs.Wrap("c.Rename", c.Rename(r.userID, a.FirmID, a.Name))
	case user.FirmUpgrade:
		_, err := c.Upgrade(r.rules, r.userID, a.FirmID)
		return errs.Wrap("c.Upgrade", err)
	case user.FirmStaff:
		return errs.Wrap("c.Staff", c.Staff(r.rules, r.userID, a.FirmID, a.Workers, a.Wage, r.now))
	case user.FirmDeposit:
		if err := c.Deposit(r.userID, a.FirmID, a.Votes); err != nil {
			return errs.Wrap("c.Deposit", err)
		}
		return r.spend(ctx, a.Votes)
	case user.FirmDividend:
		payouts, err := c.Dividend(r.userID, a.FirmID, a.Votes)
		if err != nil {
			return errs.Wrap("c.Dividend", err)
		}
		return r.pay(ctx, payouts)
	case user.FirmLiquidate:
		payouts, err := c.Liquidate(r.rules, r.userID, a.FirmID)
		if err != nil {
			return errs.Wrap("c.Liquidate", err)
		}
		return r.pay(ctx, payouts)
	case user.FirmAutomate:
		return errs.Wrap("c.SetAuto", c.SetAuto(r.userID, a.FirmID, a.Order.Good, a.Auto))
	case user.FirmOrder:
		deals, err := c.Place(r.rules, r.userID, a.FirmID, a.Order, r.now)
		r.done.Deals = deals
		return errs.Wrap("c.Place", err)
	case user.FirmCancel:
		return errs.Wrap("c.Cancel", c.Cancel(r.userID, a.OrderID))
	case user.FirmStateBuy:
		_, err := c.BuyFromState(r.rules, r.userID, a.FirmID, a.Order.Good, a.Order.Units, r.now)
		return errs.Wrap("c.BuyFromState", err)
	case user.FirmStateSell:
		_, err := c.SellToState(r.rules, r.userID, a.FirmID, a.Order.Good, a.Order.Units, r.now)
		return errs.Wrap("c.SellToState", err)
	case user.FirmIssue:
		return errs.Wrap("c.Issue", c.Issue(r.rules, r.userID, a.FirmID, a.Shares))
	case user.FirmRepair:
		_, err := c.Repair(r.rules, r.userID, a.FirmID)
		return errs.Wrap("c.Repair", err)
	case user.FirmImprove:
		_, err := c.Improve(r.rules, r.userID, a.FirmID, a.Improvement)
		return errs.Wrap("c.Improve", err)
	case user.FirmTake:
		return errs.Wrap("c.Take", c.Take(r.userID, a.FirmID, a.ContractID, r.now))
	case user.FirmDeliver:
		_, err := c.Deliver(r.userID, a.ContractID, a.Order.Units, r.now)
		return errs.Wrap("c.Deliver", err)
	case user.FirmFund:
		if err := c.Fund(r.userID, a.Votes); err != nil {
			return errs.Wrap("c.Fund", err)
		}
		return r.spend(ctx, a.Votes)
	case user.FirmWithdraw:
		if err := c.Withdraw(r.userID, a.Votes); err != nil {
			return errs.Wrap("c.Withdraw", err)
		}
		return r.pay(ctx, []user.Payout{{UserID: r.userID, Votes: a.Votes}})
	case user.FirmStockOrder:
		fill, err := c.PlaceStock(r.rules, r.userID, a.Stock, r.now)
		r.done.Fill = fill
		return errs.Wrap("c.PlaceStock", err)
	case user.FirmStockCancel:
		return errs.Wrap("c.CancelStock", c.CancelStock(r.userID, a.OrderID))
	case user.FirmTender:
		_, err := c.Tender(r.userID, a.Stock.FirmID, a.Stock.Shares, a.Stock.Price, r.now)
		return errs.Wrap("c.Tender", err)
	case user.FirmTenderSell:
		fill, err := c.SellToTender(r.rules, r.userID, a.OrderID, a.Stock.Shares, r.now)
		r.done.Fill = fill
		return errs.Wrap("c.SellToTender", err)
	case user.FirmTenderClose:
		return errs.Wrap("c.CancelTender", c.CancelTender(r.userID, a.OrderID))
	case user.FirmRoute:
		route := a.Route
		route.Good = a.Order.Good
		_, err := c.AddRoute(r.userID, a.FirmID, route)
		return errs.Wrap("c.AddRoute", err)
	case user.FirmUnroute:
		return errs.Wrap("c.RemoveRoute", c.RemoveRoute(r.userID, a.FirmID, a.RouteID))
	}

	return user.ErrUnknownFirmAction
}

func (r firmRun) found(ctx context.Context) error {
	plan, err := r.rules.Plan(r.act.Kind)
	if err != nil {
		return errs.Wrap("r.rules.Plan", err)
	}

	f, err := r.city.Found(r.rules, r.userID, r.act.Kind, r.act.Name, r.now)
	if err != nil {
		return errs.Wrap("r.city.Found", err)
	}

	r.done.FirmID = f.ID()

	return r.spend(ctx, plan.Price)
}

// spend takes votes of the caller into the treasury, for the
// enterprises.
func (r firmRun) spend(ctx context.Context, votes int64) error {
	if err := r.charge(ctx, votes); err != nil {
		return errs.Wrap("r.charge", err)
	}

	r.city.Count(votes, 0)

	return nil
}

// charge takes votes of the caller into the treasury.
func (r firmRun) charge(ctx context.Context, votes int64) error {
	left, err := r.s.wallets.Spend(ctx, r.userID, votes, firmNote(r.act.Action))
	if err != nil {
		return errs.Wrap("r.s.wallets.Spend", err)
	}

	r.done.Spent += votes
	r.done.Balance = left

	return nil
}

// pay pays the shareholders out of the treasury.
func (r firmRun) pay(ctx context.Context, payouts []user.Payout) error {
	for _, p := range payouts {
		wallet, err := r.s.wallets.Credit(ctx, p.UserID, p.Votes, 0, firmNote(r.act.Action))
		if err != nil {
			return errs.Wrap("r.s.wallets.Credit", err)
		}

		if p.UserID == r.userID {
			r.done.Got += p.Votes
			r.done.Balance = wallet.Votes
		}
		r.city.Count(0, p.Votes)
	}

	return nil
}

// Settings are the rules and the stats; user.ErrNotAdmin.
func (s *FirmService) Settings(ctx context.Context, adminID uuid.UUID) (FirmSettings, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return FirmSettings{}, errs.Wrap("checkAdmin", err)
	}

	rules, err := s.firms.Rules(ctx)
	if err != nil {
		return FirmSettings{}, errs.Wrap("s.firms.Rules", err)
	}

	city, err := s.firms.City(ctx, false)
	if err != nil {
		return FirmSettings{}, errs.Wrap("s.firms.City", err)
	}

	return FirmSettings{Rules: rules, Stats: city.Stats(), Firms: len(city.Firms())}, nil
}

// SetRules changes the enterprises; user.ErrNotAdmin.
func (s *FirmService) SetRules(ctx context.Context, adminID uuid.UUID, rules user.FirmRules) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.firms.SetRules", s.firms.SetRules(ctx, rules))
}
