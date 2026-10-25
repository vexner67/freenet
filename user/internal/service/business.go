package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// BusinessRepository keeps the businesses of the site and of its pages.
type BusinessRepository interface {
	// Rules are user.DefaultBusinessRules until some are set.
	Rules(ctx context.Context) (user.BusinessRules, error)
	SetRules(ctx context.Context, rules user.BusinessRules) error
	// Businesses are the businesses of userID in the order of
	// user.BusinessKinds.
	Businesses(ctx context.Context, userID uuid.UUID) ([]user.Business, error)
	// Business is the business kind of userID, locked till the end of
	// the transaction; user.ErrBusinessNotFound.
	Business(ctx context.Context, userID uuid.UUID, kind user.BusinessKind) (user.Business, error)
	// AddBusiness keeps a business just bought; user.ErrBusinessOwned,
	// user.ErrNotFound.
	AddBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error
	SaveBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error
	DeleteBusiness(ctx context.Context, userID uuid.UUID, kind user.BusinessKind) error
	// Count adds the votes people spent on businesses and those paid out.
	Count(ctx context.Context, spent, paid int64) error
	Stats(ctx context.Context) (user.BusinessStats, error)
}

// BusinessesView are the businesses as their owner sees them at a time.
type BusinessesView struct {
	Rules      user.BusinessRules
	Businesses []user.Business
	At         time.Time
}

// BusinessAct is an action with a business: Amount is the units of a
// restock or the votes of a deposit, Price and Staff — what a manage sets,
// Perk — the improvement an improve buys the next tier of.
type BusinessAct struct {
	Action user.BusinessAction
	Amount int64
	Price  int64
	Staff  int
	Perk   user.BusinessPerk
}

// BusinessDone is what an action did: the business after it (Closed when
// it was sold), the votes spent or got, the tax and the balance after
// them (0 when no votes moved).
type BusinessDone struct {
	Rules    user.BusinessRules
	Business user.Business
	Closed   bool
	Spent    int64
	Got      int64
	Tax      int64
	Balance  int64
	At       time.Time
}

// BusinessSettings are the rules and how the businesses do: for the
// administrators.
type BusinessSettings struct {
	Rules user.BusinessRules
	Stats user.BusinessStats
}

// BusinessService is the businesses: people buy them for votes into the
// treasury, run them — goods, price, staff, repairs, advertising, levels,
// improvements —
// and take their cash out; a business run badly runs into debt and goes
// bankrupt.
type BusinessService struct {
	userRepo   UserRepository
	businesses BusinessRepository
	wallets    WalletRepository
	tx         TxManager
	admins     user.Admins
}

func NewBusinessService(users UserRepository, businesses BusinessRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *BusinessService {
	return &BusinessService{userRepo: users, businesses: businesses, wallets: wallets, tx: tx, admins: admins}
}

// Businesses are the businesses of userID at now.
func (s *BusinessService) Businesses(ctx context.Context, userID uuid.UUID, now time.Time) (BusinessesView, error) {
	rules, err := s.businesses.Rules(ctx)
	if err != nil {
		return BusinessesView{}, errs.Wrap("s.businesses.Rules", err)
	}

	owned, err := s.businesses.Businesses(ctx, userID)
	if err != nil {
		return BusinessesView{}, errs.Wrap("s.businesses.Businesses", err)
	}

	for i, b := range owned {
		plan, err := rules.Plan(b.Kind())
		if err != nil {
			return BusinessesView{}, errs.Wrap("rules.Plan", err)
		}

		owned[i] = b.At(plan, now)
	}

	return BusinessesView{Rules: rules, Businesses: owned, At: now}, nil
}

func note(kind user.BusinessKind, action user.BusinessAction) user.VoteEntry {
	return user.Entry(user.VoteBusiness, uuid.Nil, kind.String()+":"+string(action))
}

// Buy buys kind for userID at now; user.ErrBusinessOwned,
// user.ErrNotEnoughVotes.
func (s *BusinessService) Buy(ctx context.Context, userID uuid.UUID, kind user.BusinessKind, now time.Time) (BusinessDone, error) {
	var done BusinessDone

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		rules, err := s.businesses.Rules(ctx)
		if err != nil {
			return errs.Wrap("s.businesses.Rules", err)
		}

		plan, err := rules.Plan(kind)
		if err != nil {
			return errs.Wrap("rules.Plan", err)
		}

		// Owned is told before the votes are spent; the unique key of
		// AddBusiness still keeps two buys at once from both going through.
		_, err = s.businesses.Business(ctx, userID, kind)
		if err == nil {
			return user.ErrBusinessOwned
		}
		if !errors.Is(err, user.ErrBusinessNotFound) {
			return errs.Wrap("s.businesses.Business", err)
		}

		left, err := s.wallets.Spend(ctx, userID, plan.Price, note(kind, "buy"))
		if err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		b := user.NewBusiness(plan, now)
		if err = s.businesses.AddBusiness(ctx, userID, b); err != nil {
			return errs.Wrap("s.businesses.AddBusiness", err)
		}

		done = BusinessDone{Rules: rules, Business: b, Spent: plan.Price, Balance: left, At: now}

		return errs.Wrap("s.businesses.Count", s.businesses.Count(ctx, plan.Price, 0))
	})
	if err != nil {
		return BusinessDone{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return done, nil
}

// Act does act with the business kind of userID at now. Taking votes out —
// collecting the till and selling — is for the pages that take the daily
// bonus (user.ErrBusinessNotYet).
func (s *BusinessService) Act(ctx context.Context, userID uuid.UUID, kind user.BusinessKind, act BusinessAct, now time.Time) (BusinessDone, error) {
	if act.Action == user.BusinessCollect || act.Action == user.BusinessSell {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return BusinessDone{}, errs.Wrap("s.userRepo.GetByID", err)
		}
		if err = u.CheckBusinessAt(now); err != nil {
			return BusinessDone{}, errs.Wrap("u.CheckBusinessAt", err)
		}
	}

	var done BusinessDone

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		rules, err := s.businesses.Rules(ctx)
		if err != nil {
			return errs.Wrap("s.businesses.Rules", err)
		}

		plan, err := rules.Plan(kind)
		if err != nil {
			return errs.Wrap("rules.Plan", err)
		}

		b, err := s.businesses.Business(ctx, userID, kind)
		if err != nil {
			return errs.Wrap("s.businesses.Business", err)
		}

		done = BusinessDone{Rules: rules, At: now}
		if done.Business, err = s.apply(b.At(plan, now), plan, rules, act, now, &done); err != nil {
			return errs.Wrap("s.apply", err)
		}

		if err = s.settle(ctx, userID, kind, act.Action, &done); err != nil {
			return errs.Wrap("s.settle", err)
		}

		if done.Closed {
			err = s.businesses.DeleteBusiness(ctx, userID, kind)
		} else {
			err = s.businesses.SaveBusiness(ctx, userID, done.Business)
		}
		if err != nil {
			return errs.Wrap("s.businesses.SaveBusiness", err)
		}

		return errs.Wrap("s.businesses.Count", s.businesses.Count(ctx, done.Spent, done.Got))
	})
	if err != nil {
		return BusinessDone{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return done, nil
}

// apply is b after act and the votes it costs or gives, in done.
func (s *BusinessService) apply(b user.Business, plan user.BusinessPlan, rules user.BusinessRules, act BusinessAct, now time.Time, done *BusinessDone) (user.Business, error) {
	var err error

	switch act.Action {
	case user.BusinessManage:
		b, err = b.Managed(plan, act.Price, act.Staff)
	case user.BusinessRestock:
		b, done.Spent, err = b.Restocked(plan, act.Amount)
	case user.BusinessUpgrade:
		b, done.Spent, err = b.Upgraded(plan)
	case user.BusinessRepair:
		b, done.Spent, err = b.Repaired(plan)
	case user.BusinessImprove:
		b, done.Spent, err = b.Improved(plan, act.Perk)
	case user.BusinessAdvertise:
		b, done.Spent, err = b.Advertised(plan, now)
	case user.BusinessDeposit:
		b, err = b.Deposited(act.Amount)
		done.Spent = act.Amount
	case user.BusinessCollect:
		b, done.Got, done.Tax, err = b.Collected(rules.Tax())
	case user.BusinessSell:
		done.Got, err = b.Resale()
		done.Closed = true
	default:
		err = user.ErrUnknownAction
	}
	if err != nil {
		return user.Business{}, errs.Wrap("b."+string(act.Action), err)
	}

	return b, nil
}

// settle moves the votes of done: what is spent goes to the treasury,
// what is got comes out of it.
func (s *BusinessService) settle(ctx context.Context, userID uuid.UUID, kind user.BusinessKind, action user.BusinessAction, done *BusinessDone) error {
	if done.Spent > 0 {
		left, err := s.wallets.Spend(ctx, userID, done.Spent, note(kind, action))
		if err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		done.Balance = left
	}

	if done.Got > 0 {
		wallet, err := s.wallets.Credit(ctx, userID, done.Got, 0, note(kind, action))
		if err != nil {
			return errs.Wrap("s.wallets.Credit", err)
		}

		done.Balance = wallet.Votes
	}

	return nil
}

// Settings are the rules and the stats; user.ErrNotAdmin.
func (s *BusinessService) Settings(ctx context.Context, adminID uuid.UUID) (BusinessSettings, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return BusinessSettings{}, errs.Wrap("checkAdmin", err)
	}

	rules, err := s.businesses.Rules(ctx)
	if err != nil {
		return BusinessSettings{}, errs.Wrap("s.businesses.Rules", err)
	}

	stats, err := s.businesses.Stats(ctx)
	if err != nil {
		return BusinessSettings{}, errs.Wrap("s.businesses.Stats", err)
	}

	return BusinessSettings{Rules: rules, Stats: stats}, nil
}

// SetRules changes the businesses; user.ErrNotAdmin.
func (s *BusinessService) SetRules(ctx context.Context, adminID uuid.UUID, rules user.BusinessRules) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.businesses.SetRules", s.businesses.SetRules(ctx, rules))
}
