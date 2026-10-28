package grpc

import (
	"context"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
)

// WorldService takes and pays the votes of the states of «Мир».
type WorldService interface {
	ChargeVotes(ctx context.Context, v service.WorldVotes) (int64, error)
	PayVotes(ctx context.Context, v service.WorldVotes) (int64, error)
}

func (h *UserHandler) ChargeVotes(ctx context.Context, req *userv1.ChargeVotesRequest) (*userv1.ChargeVotesResponse, error) {
	v, err := worldVotes(req.GetOpId(), req.GetUserId(), req.GetVotes(), req.GetNote())
	if err != nil {
		return nil, err
	}
	v.Settled = req.GetSettled()

	left, err := h.world.ChargeVotes(ctx, v)
	if err != nil {
		return nil, err
	}

	return &userv1.ChargeVotesResponse{Votes: left}, nil
}

func (h *UserHandler) PayVotes(ctx context.Context, req *userv1.PayVotesRequest) (*userv1.PayVotesResponse, error) {
	v, err := worldVotes(req.GetOpId(), req.GetUserId(), req.GetVotes(), req.GetNote())
	if err != nil {
		return nil, err
	}

	votes, err := h.world.PayVotes(ctx, v)
	if err != nil {
		return nil, err
	}

	return &userv1.PayVotesResponse{Votes: votes}, nil
}

func worldVotes(opID, userID string, votes int64, note string) (service.WorldVotes, error) {
	op, err := parseID(opID, "op id")
	if err != nil {
		return service.WorldVotes{}, err
	}

	id, err := parseID(userID, "user id")
	if err != nil {
		return service.WorldVotes{}, err
	}

	return service.WorldVotes{OpID: op, UserID: id, Votes: votes, Note: note}, nil
}
