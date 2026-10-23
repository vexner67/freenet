package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/protobuf/types/known/emptypb"
)

// PaidService sells the photos of paid posts and messages to those who
// ask votes for them.
type PaidService interface {
	Bought(ctx context.Context, buyerID uuid.UUID, postIDs []uuid.UUID) ([]uuid.UUID, error)
	BuyPost(ctx context.Context, buyerID, postID, authorID uuid.UUID, price int64) (int64, error)
	Terms(ctx context.Context, senderID, recipientID uuid.UUID, wroteFirst bool) (service.MessageTerms, error)
	PayMessage(ctx context.Context, senderID, recipientID uuid.UUID, wroteFirst bool, price int64) (user.MessagePayment, int64, error)
	RefundMessage(ctx context.Context, senderID, paymentID uuid.UUID) error
}

func (h *UserHandler) GetBoughtPosts(ctx context.Context, req *userv1.GetBoughtPostsRequest) (*userv1.GetBoughtPostsResponse, error) {
	buyerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(req.GetPostIds()))
	for _, s := range req.GetPostIds() {
		id, err := parseID(s, "post id")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	bought, err := h.paid.Bought(ctx, buyerID, ids)
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetBoughtPostsResponse{PostIds: make([]string, 0, len(bought))}
	for _, id := range bought {
		resp.PostIds = append(resp.PostIds, id.String())
	}

	return resp, nil
}

func (h *UserHandler) BuyPost(ctx context.Context, req *userv1.BuyPostRequest) (*userv1.BuyPostResponse, error) {
	buyerID, authorID, err := callerAnd(ctx, req.GetAuthorId())
	if err != nil {
		return nil, err
	}

	postID, err := parseID(req.GetPostId(), "post id")
	if err != nil {
		return nil, err
	}

	left, err := h.paid.BuyPost(ctx, buyerID, postID, authorID, req.GetPrice())
	if err != nil {
		return nil, err
	}

	return &userv1.BuyPostResponse{Votes: left}, nil
}

func (h *UserHandler) CanMessage(ctx context.Context, req *userv1.CanMessageRequest) (*userv1.CanMessageResponse, error) {
	senderID, recipientID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	terms, err := h.paid.Terms(ctx, senderID, recipientID, req.GetWroteFirst())
	if err != nil {
		return nil, err
	}

	return &userv1.CanMessageResponse{Allowed: terms.Allowed, PrimeOnly: terms.PrimeOnly, Price: terms.Price}, nil
}

func (h *UserHandler) PayMessage(ctx context.Context, req *userv1.PayMessageRequest) (*userv1.PayMessageResponse, error) {
	senderID, recipientID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	payment, left, err := h.paid.PayMessage(ctx, senderID, recipientID, req.GetWroteFirst(), req.GetPrice())
	if err != nil {
		return nil, err
	}

	resp := &userv1.PayMessageResponse{Votes: left}
	if payment.ID != uuid.Nil {
		resp.PaymentId = payment.ID.String()
	}

	return resp, nil
}

func (h *UserHandler) RefundMessage(ctx context.Context, req *userv1.RefundMessageRequest) (*emptypb.Empty, error) {
	senderID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	paymentID, err := parseID(req.GetPaymentId(), "payment id")
	if err != nil {
		return nil, err
	}

	if err = h.paid.RefundMessage(ctx, senderID, paymentID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
