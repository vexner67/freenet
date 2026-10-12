package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type WalletService interface {
	IsAdmin(u *user.User) bool
	Transfer(ctx context.Context, senderID, recipientID uuid.UUID, votes user.Votes) (int64, error)
	Redeem(ctx context.Context, userID uuid.UUID, token user.VoucherToken) (service.Redeemed, error)
	CreateVoucher(ctx context.Context, adminID uuid.UUID, d service.VoucherDraft) (*user.Voucher, error)
	ListVouchers(ctx context.Context, adminID, cursor uuid.UUID, size int) (service.VoucherPage, error)
	DeleteVoucher(ctx context.Context, adminID, voucherID uuid.UUID) error
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

	left, err := h.walletService.Transfer(ctx, senderID, recipientID, votes)
	if err != nil {
		return nil, err
	}

	return &userv1.TransferVotesResponse{Votes: left}, nil
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
