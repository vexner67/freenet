package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
)

// Bank is the bank of votes, which the user service keeps with them:
// deposits and loans.
type Bank struct {
	api userv1.UserServiceClient
}

func NewBank(api userv1.UserServiceClient) *Bank {
	return &Bank{
		api: api,
	}
}

func (b *Bank) Get(ctx context.Context, accessToken string) (model.Bank, error) {
	resp, err := b.api.GetBank(withToken(ctx, accessToken), &userv1.GetBankRequest{})
	if err != nil {
		return model.Bank{}, errs.Wrap("b.api.GetBank", fromStatus(err))
	}

	return fromProtoBank(resp), nil
}

type bankCall func(ctx context.Context, in *userv1.BankAmountRequest, opts ...grpc.CallOption) (*userv1.BankMoveResponse, error)

// Move does action — "deposit", "withdraw", "loan" or "repay" — with
// amount votes.
func (b *Bank) Move(ctx context.Context, accessToken, action string, amount int64) (model.BankMove, error) {
	calls := map[string]bankCall{
		"deposit":  b.api.PutDeposit,
		"withdraw": b.api.TakeDeposit,
		"loan":     b.api.TakeLoan,
		"repay":    b.api.RepayLoan,
	}

	call, ok := calls[action]
	if !ok {
		return model.BankMove{}, failure.New(failure.NotFound, "unknown action of the bank")
	}

	resp, err := call(withToken(ctx, accessToken), &userv1.BankAmountRequest{Amount: amount})
	if err != nil {
		return model.BankMove{}, errs.Wrap("call", fromStatus(err))
	}

	return model.BankMove{Votes: resp.GetVotes(), Balance: resp.GetBalance(), Bank: fromProtoBank(resp.GetBank())}, nil
}

func (b *Bank) Settings(ctx context.Context, accessToken string) (model.BankSettings, model.BankStats, error) {
	resp, err := b.api.GetBankSettings(withToken(ctx, accessToken), &userv1.GetBankSettingsRequest{})
	if err != nil {
		return model.BankSettings{}, model.BankStats{}, errs.Wrap("b.api.GetBankSettings", fromStatus(err))
	}

	stats := model.BankStats{
		Deposits: resp.GetDeposits(), Depositors: resp.GetDepositors(), Debts: resp.GetDebts(), Debtors: resp.GetDebtors(),
	}

	return fromProtoBankSettings(resp.GetSettings()), stats, nil
}

func (b *Bank) Set(ctx context.Context, accessToken string, s model.BankSettings) error {
	req := &userv1.BankSettings{DepositMost: s.DepositMost, LoanMost: s.LoanMost}

	ints := []struct {
		n    int
		what string
		to   *int32
	}{
		{s.DepositRate, "the rate of deposits", &req.DepositRate},
		{s.LoanRate, "the rate of loans", &req.LoanRate},
		{s.LoanDays, "the term of a loan", &req.LoanDays},
		{s.PenaltyRate, "the rate of overdue loans", &req.PenaltyRate},
	}
	for _, i := range ints {
		n, err := int32Of(i.n, i.what)
		if err != nil {
			return err
		}

		*i.to = n
	}

	if _, err := b.api.SetBankSettings(withToken(ctx, accessToken), &userv1.SetBankSettingsRequest{Settings: req}); err != nil {
		return errs.Wrap("b.api.SetBankSettings", fromStatus(err))
	}

	return nil
}

func fromProtoBankCard(resp *userv1.BankCardResponse) model.BankCard {
	c := resp.GetCard()

	return model.BankCard{Number: c.GetNumber(), Cover: c.GetCover(), IssuedAt: c.GetIssuedAt().AsTime()}
}

func (b *Bank) Card(ctx context.Context, accessToken string) (model.BankCard, error) {
	resp, err := b.api.GetBankCard(withToken(ctx, accessToken), &userv1.GetBankCardRequest{})
	if err != nil {
		return model.BankCard{}, errs.Wrap("b.api.GetBankCard", fromStatus(err))
	}

	return fromProtoBankCard(resp), nil
}

func (b *Bank) IssueCard(ctx context.Context, accessToken string) (model.BankCard, error) {
	resp, err := b.api.IssueBankCard(withToken(ctx, accessToken), &userv1.IssueBankCardRequest{})
	if err != nil {
		return model.BankCard{}, errs.Wrap("b.api.IssueBankCard", fromStatus(err))
	}

	return fromProtoBankCard(resp), nil
}

func (b *Bank) SetCardCover(ctx context.Context, accessToken, cover string) (model.BankCard, error) {
	resp, err := b.api.SetBankCardCover(withToken(ctx, accessToken), &userv1.SetBankCardCoverRequest{Cover: cover})
	if err != nil {
		return model.BankCard{}, errs.Wrap("b.api.SetBankCardCover", fromStatus(err))
	}

	return fromProtoBankCard(resp), nil
}

func fromProtoBankSettings(s *userv1.BankSettings) model.BankSettings {
	return model.BankSettings{
		DepositRate: int(s.GetDepositRate()),
		DepositMost: s.GetDepositMost(),
		LoanRate:    int(s.GetLoanRate()),
		LoanMost:    s.GetLoanMost(),
		LoanDays:    int(s.GetLoanDays()),
		PenaltyRate: int(s.GetPenaltyRate()),
	}
}

func fromProtoBank(s *userv1.BankState) model.Bank {
	a := s.GetAccount()
	account := model.BankAccount{Deposit: a.GetDeposit(), Debt: a.GetDebt(), Borrowed: a.GetBorrowed(), Overdue: a.GetOverdue()}
	if a.GetDueAt() != nil {
		account.DueAt = a.GetDueAt().AsTime()
	}

	return model.Bank{Settings: fromProtoBankSettings(s.GetSettings()), Account: account}
}
