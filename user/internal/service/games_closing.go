package service

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// ClosingGameAssets is the inventory used to prepare a closing plan.
type ClosingGameAssets interface {
	Assets(ctx context.Context, at time.Time) (user.ClosingAssets, error)
	Existing(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
}

// ClosingGamePlans keeps the frozen settlement and its progress.
type ClosingGamePlans interface {
	Lock(ctx context.Context) error
	Plan(ctx context.Context) (ClosingReport, bool, error)
	SavePlan(ctx context.Context, at time.Time, report ClosingReport) error
	Paid(ctx context.Context, id uuid.UUID) (bool, error)
	MarkPaid(ctx context.Context, id uuid.UUID) error
}

type ClosingAccount struct {
	UserID  uuid.UUID
	Amounts map[string]int64
	Total   int64
}

type ClosingReport struct {
	Accounts []ClosingAccount
	ByReason map[string]int64
	Total    int64
}

type GamesService struct {
	assets  ClosingGameAssets
	plans   ClosingGamePlans
	wallets WalletRepository
	tx      TxManager
}

func NewGamesService(assets ClosingGameAssets, plans ClosingGamePlans, wallets WalletRepository, tx TxManager) *GamesService {
	return &GamesService{assets: assets, plans: plans, wallets: wallets, tx: tx}
}

// Close calculates a preview, or freezes the plan and pays each account
// in its own transaction. A restart reuses the original plan.
func (s *GamesService) Close(ctx context.Context, run bool, clock func() time.Time) (ClosingReport, error) {
	var report ClosingReport
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.plans.Lock(ctx); err != nil {
			return errs.Wrap("s.plans.Lock", err)
		}
		stored, frozen, err := s.plans.Plan(ctx)
		if err != nil || frozen {
			report = stored
			return errs.Wrap("s.plans.Plan", err)
		}
		at := clock().UTC()
		report, err = s.calculate(ctx, at)
		if err != nil || !run {
			return errs.Wrap("s.calculate", err)
		}

		return errs.Wrap("s.plans.SavePlan", s.plans.SavePlan(ctx, at, report))
	})
	if err != nil || !run {
		return report, errs.Wrap("s.tx.WithinTx", err)
	}
	for _, account := range report.Accounts {
		if err = s.pay(ctx, account); err != nil {
			return report, errs.Wrap("s.pay", err)
		}
	}

	return report, nil
}

func (s *GamesService) calculate(ctx context.Context, at time.Time) (ClosingReport, error) {
	assets, err := s.assets.Assets(ctx, at)
	if err != nil {
		return ClosingReport{}, errs.Wrap("s.assets.Assets", err)
	}
	payouts, err := assets.ClosingPayouts(at)
	if err != nil {
		return ClosingReport{}, errs.Wrap("assets.ClosingPayouts", err)
	}

	accounts := make(map[uuid.UUID]map[string]int64)
	for _, p := range payouts {
		if p.UserID == uuid.Nil || p.UserID == user.MarketID || p.Votes == 0 {
			continue
		}
		if accounts[p.UserID] == nil {
			accounts[p.UserID] = make(map[string]int64)
		}
		amounts := accounts[p.UserID]
		amounts[p.Reason], err = user.AddClosingVotes(amounts[p.Reason], p.Votes)
		if err != nil {
			return ClosingReport{}, errs.Wrap("user.AddClosingVotes", err)
		}
	}
	existing, err := s.assets.Existing(ctx, slices.Collect(maps.Keys(accounts)))
	if err != nil {
		return ClosingReport{}, errs.Wrap("s.assets.Existing", err)
	}

	report := ClosingReport{ByReason: make(map[string]int64)}
	for _, id := range existing {
		if err := report.add(id, accounts[id]); err != nil {
			return ClosingReport{}, errs.Wrap("report.add", err)
		}
	}
	slices.SortFunc(report.Accounts, func(a, b ClosingAccount) int {
		if order := cmp.Compare(b.Total, a.Total); order != 0 {
			return order
		}
		return cmp.Compare(a.UserID.String(), b.UserID.String())
	})

	return report, nil
}

func (r *ClosingReport) add(id uuid.UUID, amounts map[string]int64) error {
	account := ClosingAccount{UserID: id, Amounts: amounts}
	for reason, votes := range amounts {
		total, err := user.AddClosingVotes(account.Total, votes)
		if err != nil {
			return errs.Wrap("user.AddClosingVotes", err)
		}
		account.Total = total
		total, err = user.AddClosingVotes(r.ByReason[reason], votes)
		if err != nil {
			return errs.Wrap("user.AddClosingVotes", err)
		}
		r.ByReason[reason] = total
	}
	total, err := user.AddClosingVotes(r.Total, account.Total)
	if err != nil {
		return errs.Wrap("user.AddClosingVotes", err)
	}
	r.Total = total
	r.Accounts = append(r.Accounts, account)

	return nil
}

func (s *GamesService) pay(ctx context.Context, account ClosingAccount) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.plans.Lock(ctx); err != nil {
			return errs.Wrap("s.plans.Lock", err)
		}
		paid, err := s.plans.Paid(ctx, account.UserID)
		if err != nil || paid {
			return errs.Wrap("s.plans.Paid", err)
		}
		for _, reason := range slices.Sorted(maps.Keys(account.Amounts)) {
			entry := user.Entry(user.VoteClosing, uuid.Nil, reason)
			if _, err := s.wallets.Credit(ctx, account.UserID, account.Amounts[reason], 0, entry); err != nil {
				return errs.Wrap("s.wallets.Credit", err)
			}
		}

		return errs.Wrap("s.plans.MarkPaid", s.plans.MarkPaid(ctx, account.UserID))
	})
	return errs.Wrap("s.tx.WithinTx", err)
}
