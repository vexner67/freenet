package grpc

import (
	"context"
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

type WalletService interface {
	IsAdmin(u *user.User) bool
	Transfer(ctx context.Context, senderID, recipientID uuid.UUID, votes user.Votes, note user.TransferNote) (left, fee int64, err error)
	Statement(ctx context.Context, userID, cursor uuid.UUID, flow user.VoteFlow, size int, now time.Time) (service.StatementPage, error)
	Redeem(ctx context.Context, userID uuid.UUID, token user.VoucherToken) (service.Redeemed, error)
	CreateVoucher(ctx context.Context, adminID uuid.UUID, d service.VoucherDraft) (*user.Voucher, error)
	ListVouchers(ctx context.Context, adminID, cursor uuid.UUID, size int) (service.VoucherPage, error)
	DeleteVoucher(ctx context.Context, adminID, voucherID uuid.UUID) error
	Rate(ctx context.Context) (user.VoteRate, error)
	SetRate(ctx context.Context, adminID uuid.UUID, rate user.VoteRate) error
	Treasury(ctx context.Context) (user.Treasury, error)
	Bonus(ctx context.Context, userID uuid.UUID, now time.Time) (service.Bonus, error)
	ClaimBonus(ctx context.Context, userID uuid.UUID, now time.Time) (service.Redeemed, error)
	RaiseRating(ctx context.Context, userID uuid.UUID, raise user.RatingRaise) (user.Wallet, error)
}

func (h *UserHandler) TransferVotes(ctx context.Context, req *userv1.TransferVotesRequest) (*userv1.TransferVotesResponse, error) {
	senderID, recipientID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	votes, err := user.NewVotes(req.GetVotes())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	note, err := user.NewTransferNote(req.GetNote())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	left, fee, err := h.walletService.Transfer(ctx, senderID, recipientID, votes, note)
	if err != nil {
		return nil, err
	}

	return &userv1.TransferVotesResponse{Votes: left, Fee: fee}, nil
}

func (h *UserHandler) ListVoteOperations(ctx context.Context, req *userv1.ListVoteOperationsRequest) (*userv1.ListVoteOperationsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var cursor uuid.UUID
	if req.GetCursor() != "" {
		if cursor, err = parseID(req.GetCursor(), "cursor"); err != nil {
			return nil, err
		}
	}

	flow, err := user.NewVoteFlow(req.GetFlow())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	page, err := h.walletService.Statement(ctx, userID, cursor, flow, int(req.GetLimit()), time.Now())
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListVoteOperationsResponse{
		Operations: make([]*userv1.VoteOperation, 0, len(page.Operations)),
		Income:     page.Income,
		Expense:    page.Expense,
	}

	for _, op := range page.Operations {
		resp.Operations = append(resp.Operations, toProtoVoteOperation(op))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *UserHandler) RedeemVoucher(ctx context.Context, req *userv1.RedeemVoucherRequest) (*userv1.RedeemVoucherResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	token, err := user.NewVoucherToken(req.GetToken())
	if err != nil {
		// A serial number of a wrong form names no voucher either.
		return nil, user.ErrVoucherInvalid
	}

	redeemed, err := h.walletService.Redeem(ctx, userID, token)
	if err != nil {
		return nil, err
	}

	return &userv1.RedeemVoucherResponse{
		Votes:   redeemed.Votes,
		Rating:  int32(redeemed.Rating), //nolint:gosec // a voucher gives up to 1000
		Balance: redeemed.Wallet.Votes,
	}, nil
}

func (h *UserHandler) CreateVoucher(ctx context.Context, req *userv1.CreateVoucherRequest) (*userv1.CreateVoucherResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	draft := service.VoucherDraft{
		Votes:  req.GetVotes(),
		Rating: int(req.GetRating()),
		Usages: int(req.GetUsages()),
	}

	if req.GetToken() != "" {
		if draft.Token, err = user.NewVoucherToken(req.GetToken()); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	voucher, err := h.walletService.CreateVoucher(ctx, adminID, draft)
	if err != nil {
		return nil, err
	}

	return &userv1.CreateVoucherResponse{Voucher: toProtoVoucher(voucher)}, nil
}

func (h *UserHandler) ListVouchers(ctx context.Context, req *userv1.ListVouchersRequest) (*userv1.ListVouchersResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var cursor uuid.UUID
	if req.GetCursor() != "" {
		if cursor, err = parseID(req.GetCursor(), "cursor"); err != nil {
			return nil, err
		}
	}

	page, err := h.walletService.ListVouchers(ctx, adminID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListVouchersResponse{Vouchers: make([]*userv1.Voucher, 0, len(page.Vouchers))}
	for _, v := range page.Vouchers {
		resp.Vouchers = append(resp.Vouchers, toProtoVoucher(v))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *UserHandler) DeleteVoucher(ctx context.Context, req *userv1.DeleteVoucherRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	voucherID, err := parseID(req.GetVoucherId(), "voucher id")
	if err != nil {
		return nil, err
	}

	if err = h.walletService.DeleteVoucher(ctx, adminID, voucherID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func toProtoVoteOperation(op user.VoteOperation) *userv1.VoteOperation {
	resp := &userv1.VoteOperation{
		Id:        op.ID.String(),
		Kind:      string(op.Kind),
		Amount:    op.Amount,
		Balance:   op.Balance,
		Note:      op.Note,
		CreatedAt: timestamppb.New(op.At),
	}

	if op.PeerID != uuid.Nil {
		resp.PeerId = op.PeerID.String()
	}

	return resp
}

func toProtoVoucher(v *user.Voucher) *userv1.Voucher {
	return &userv1.Voucher{
		Id:          v.ID().String(),
		Token:       v.Token().String(),
		Votes:       v.Votes(),
		Rating:      int32(v.Rating()),      //nolint:gosec // up to 1000
		UsagesLeft:  int32(v.UsagesLeft()),  //nolint:gosec // up to 1000000
		Activations: int32(v.Activations()), //nolint:gosec // counts of accounts fit in int32
	}
}

func (h *UserHandler) GetVoteRate(ctx context.Context, _ *userv1.GetVoteRateRequest) (*userv1.GetVoteRateResponse, error) {
	rate, err := h.walletService.Rate(ctx)
	if err != nil {
		return nil, err
	}

	return &userv1.GetVoteRateResponse{Kopecks: rate.Kopecks()}, nil
}

func (h *UserHandler) SetVoteRate(ctx context.Context, req *userv1.SetVoteRateRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	rate, err := user.NewVoteRate(req.GetKopecks())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.walletService.SetRate(ctx, adminID, rate); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) GetTreasury(ctx context.Context, _ *userv1.GetTreasuryRequest) (*userv1.GetTreasuryResponse, error) {
	treasury, err := h.walletService.Treasury(ctx)
	if err != nil {
		return nil, err
	}

	return &userv1.GetTreasuryResponse{
		Votes:       treasury.Votes,
		Outstanding: treasury.Outstanding,
		Bonus:       user.MaxBonus,
		MaxWin:      user.MaxWin,
		Held:        treasury.Held,
	}, nil
}

func (h *UserHandler) GetBonus(ctx context.Context, _ *userv1.GetBonusRequest) (*userv1.GetBonusResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	bonus, err := h.walletService.Bonus(ctx, userID, time.Now())
	if err != nil {
		return nil, err
	}

	return &userv1.GetBonusResponse{Votes: bonus.Votes, Taken: bonus.Taken, Eligible: bonus.Eligible}, nil
}

func (h *UserHandler) ClaimBonus(ctx context.Context, _ *userv1.ClaimBonusRequest) (*userv1.ClaimBonusResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	claimed, err := h.walletService.ClaimBonus(ctx, userID, time.Now())
	if err != nil {
		return nil, err
	}

	return &userv1.ClaimBonusResponse{Votes: claimed.Votes, Balance: claimed.Wallet.Votes}, nil
}

func (h *UserHandler) RaiseRating(ctx context.Context, req *userv1.RaiseRatingRequest) (*userv1.RaiseRatingResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	raise, err := user.NewRatingRaise(int(req.GetPercent()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	wallet, err := h.walletService.RaiseRating(ctx, userID, raise)
	if err != nil {
		return nil, err
	}

	return &userv1.RaiseRatingResponse{Balance: wallet.Votes, Rating: int32(wallet.Rating)}, nil //nolint:gosec // at most user.MaxRating and what vouchers gave
}
