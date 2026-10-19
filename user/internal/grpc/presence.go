package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PresenceService interface {
	MarkOnline(ctx context.Context, id uuid.UUID) error
	Statistics(ctx context.Context) (service.Statistics, error)
}

// GetStatistics counts the people of the site, for its page "о сайте".
func (h *UserHandler) GetStatistics(ctx context.Context, _ *userv1.GetStatisticsRequest) (*userv1.GetStatisticsResponse, error) {
	stats, err := h.presenceService.Statistics(ctx)
	if err != nil {
		return nil, err
	}

	return &userv1.GetStatisticsResponse{
		Users:    int32(stats.Users),    //nolint:gosec // counts of people fit in int32
		Online:   int32(stats.Online),   //nolint:gosec // counts of people fit in int32
		Verified: int32(stats.Verified), //nolint:gosec // counts of people fit in int32
	}, nil
}

// MarkOnline: the caller is on the site now ("Онлайн").
func (h *UserHandler) MarkOnline(ctx context.Context, _ *userv1.MarkOnlineRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err = h.presenceService.MarkOnline(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
