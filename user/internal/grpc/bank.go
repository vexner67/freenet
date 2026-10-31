package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type BankService interface {
	Bank(ctx context.Context, userID uuid.UUID, now time.Time) (service.BankView, error)
	PutDeposit(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (service.Banked, error)
	TakeDeposit(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (service.Banked, error)
	Borrow(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (service.Banked, error)
	Repay(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (service.Banked, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.BankSettings, error)
	SetBank(ctx context.Context, adminID uuid.UUID, b user.Bank) error
}

func toProtoBankSettings(b user.Bank) *userv1.BankSettings {
	return &userv1.BankSettings{
		DepositRate: int32(b.DepositRate), //nolint:gosec // at most user.BankOdds
		DepositMost: b.DepositMost,
		LoanRate:    int32(b.LoanRate), //nolint:gosec // at most user.BankOdds
		LoanMost:    b.LoanMost,
		LoanDays:    int32(b.LoanDays),    //nolint:gosec // at most user.MaxBankDays
		PenaltyRate: int32(b.PenaltyRate), //nolint:gosec // at most user.BankOdds
	}
}

func fromProtoBankSettings(s *userv1.BankSettings) (user.Bank, error) {
	return user.NewBank(user.Bank{
		DepositRate: int(s.GetDepositRate()),
		DepositMost: s.GetDepositMost(),
		LoanRate:    int(s.GetLoanRate()),
		LoanMost:    s.GetLoanMost(),
		LoanDays:    int(s.GetLoanDays()),
		PenaltyRate: int(s.GetPenaltyRate()),
	})
}

func toProtoBankState(v service.BankView, now time.Time) *userv1.BankState {
	a := v.Account
	account := &userv1.BankAccount{
		Deposit:  a.Deposit(),
		Debt:     a.Debt(),
		Borrowed: a.Borrowed(),
		Overdue:  a.OverdueAt(now),
	}
	if a.HasLoan() {
		account.DueAt = timestamppb.New(a.DueAt())
	}

	return &userv1.BankState{Settings: toProtoBankSettings(v.Bank), Account: account}
}

func (h *UserHandler) GetBank(ctx context.Context, _ *userv1.GetBankRequest) (*userv1.BankState, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	v, err := h.banks.Bank(ctx, userID, now)
	if err != nil {
		return nil, err
	}

	return toProtoBankState(v, now), nil
}

type bankMove func(ctx context.Context, userID uuid.UUID, votes int64, now time.Time) (service.Banked, error)

func (h *UserHandler) moveBank(ctx context.Context, req *userv1.BankAmountRequest, move bankMove) (*userv1.BankMoveResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	got, err := move(ctx, userID, req.GetAmount(), now)
	if err != nil {
		return nil, err
	}

	return &userv1.BankMoveResponse{Votes: got.Votes, Balance: got.Balance, Bank: toProtoBankState(got.View, now)}, nil
}

func (h *UserHandler) PutDeposit(ctx context.Context, req *userv1.BankAmountRequest) (*userv1.BankMoveResponse, error) {
	return h.moveBank(ctx, req, h.banks.PutDeposit)
}

func (h *UserHandler) TakeDeposit(ctx context.Context, req *userv1.BankAmountRequest) (*userv1.BankMoveResponse, error) {
	return h.moveBank(ctx, req, h.banks.TakeDeposit)
}

func (h *UserHandler) TakeLoan(ctx context.Context, req *userv1.BankAmountRequest) (*userv1.BankMoveResponse, error) {
	return h.moveBank(ctx, req, h.banks.Borrow)
}

func (h *UserHandler) RepayLoan(ctx context.Context, req *userv1.BankAmountRequest) (*userv1.BankMoveResponse, error) {
	return h.moveBank(ctx, req, h.banks.Repay)
}

func (h *UserHandler) GetBankSettings(ctx context.Context, _ *userv1.GetBankSettingsRequest) (*userv1.GetBankSettingsResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	s, err := h.banks.Settings(ctx, adminID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetBankSettingsResponse{
		Settings:   toProtoBankSettings(s.Bank),
		Deposits:   s.Stats.Deposits,
		Depositors: s.Stats.Depositors,
		Debts:      s.Stats.Debts,
		Debtors:    s.Stats.Debtors,
	}, nil
}

func (h *UserHandler) SetBankSettings(ctx context.Context, req *userv1.SetBankSettingsRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	b, err := fromProtoBankSettings(req.GetSettings())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.banks.SetBank(ctx, adminID, b); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// CardService is the virtual cards of the pages.
type CardService interface {
	Card(ctx context.Context, userID uuid.UUID) (user.BankCard, error)
	Issue(ctx context.Context, userID uuid.UUID, now time.Time) (user.BankCard, error)
	SetCover(ctx context.Context, userID uuid.UUID, cover user.CardCover) (user.BankCard, error)
}

func toProtoBankCard(c user.BankCard) *userv1.BankCardResponse {
	return &userv1.BankCardResponse{Card: &userv1.BankCard{
		Number:   c.Number().String(),
		Cover:    c.Cover().String(),
		IssuedAt: timestamppb.New(c.IssuedAt()),
	}}
}

func (h *UserHandler) GetBankCard(ctx context.Context, _ *userv1.GetBankCardRequest) (*userv1.BankCardResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	c, err := h.cards.Card(ctx, userID)
	if errors.Is(err, user.ErrNoCard) {
		return &userv1.BankCardResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	return toProtoBankCard(c), nil
}

func (h *UserHandler) IssueBankCard(ctx context.Context, _ *userv1.IssueBankCardRequest) (*userv1.BankCardResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	c, err := h.cards.Issue(ctx, userID, time.Now())
	if err != nil {
		return nil, err
	}

	return toProtoBankCard(c), nil
}

func (h *UserHandler) SetBankCardCover(ctx context.Context, req *userv1.SetBankCardCoverRequest) (*userv1.BankCardResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cover, err := user.NewCardCover(req.GetCover())
	if err != nil {
		return nil, err
	}

	c, err := h.cards.SetCover(ctx, userID, cover)
	if err != nil {
		return nil, err
	}

	return toProtoBankCard(c), nil
}
