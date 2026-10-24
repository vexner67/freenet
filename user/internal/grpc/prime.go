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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PrimeService interface {
	Prices(ctx context.Context) (user.PrimePrices, error)
	SetPrices(ctx context.Context, adminID uuid.UUID, prices user.PrimePrices) error
	Buy(ctx context.Context, buyerID, recipientID uuid.UUID, period user.PrimePeriod) (service.PrimePurchase, error)
	SetInvisible(ctx context.Context, userID uuid.UUID, invisible bool) error
	SetStyle(ctx context.Context, userID uuid.UUID, style user.PrimeStyle) error
}

func (h *UserHandler) GetPrime(ctx context.Context, _ *userv1.GetPrimeRequest) (*userv1.GetPrimeResponse, error) {
	prices, err := h.primes.Prices(ctx)
	if err != nil {
		return nil, err
	}

	return &userv1.GetPrimeResponse{Prices: toProtoPrimePrices(prices)}, nil
}

func (h *UserHandler) BuyPrime(ctx context.Context, req *userv1.BuyPrimeRequest) (*userv1.BuyPrimeResponse, error) {
	buyerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	recipientID := buyerID
	if req.GetUserId() != "" {
		if _, recipientID, err = callerAnd(ctx, req.GetUserId()); err != nil {
			return nil, err
		}
	}

	period, err := user.NewPrimePeriod(req.GetPeriod())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	bought, err := h.primes.Buy(ctx, buyerID, recipientID, period)
	if err != nil {
		return nil, err
	}

	return &userv1.BuyPrimeResponse{Until: timestamppb.New(bought.Until), Balance: bought.Balance}, nil
}

func (h *UserHandler) SetPrimePrices(ctx context.Context, req *userv1.SetPrimePricesRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	p := req.GetPrices()

	prices, err := user.NewPrimePrices(p.GetMonth(), p.GetQuarter(), p.GetYear())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.primes.SetPrices(ctx, adminID, prices); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) SetPrimeInvisible(ctx context.Context, req *userv1.SetPrimeInvisibleRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err = h.primes.SetInvisible(ctx, userID, req.GetInvisible()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) SetPrimeStyle(ctx context.Context, req *userv1.SetPrimeStyleRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	st := req.GetStyle()

	style, err := user.NewPrimeStyle(st.GetName(), st.GetFrame(), st.GetBackground(), st.GetMood())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.primes.SetStyle(ctx, userID, style); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
