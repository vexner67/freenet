package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type IgnoreService interface {
	Ignore(ctx context.Context, userID, sourceID uuid.UUID) error
	Unignore(ctx context.Context, userID uuid.UUID, sourceIDs []uuid.UUID) error
	Ignored(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

func (h *UserHandler) IgnoreSource(ctx context.Context, req *userv1.IgnoreSourceRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	sourceID, err := parseID(req.GetSourceId(), "source id")
	if err != nil {
		return nil, err
	}

	if err = h.ignoreService.Ignore(ctx, userID, sourceID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) UnignoreSources(ctx context.Context, req *userv1.UnignoreSourcesRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	if n := len(req.GetSourceIds()); n == 0 || n > user.MaxUnignored {
		return nil, status.Error(codes.InvalidArgument, "invalid number of sources")
	}

	sourceIDs := make([]uuid.UUID, 0, len(req.GetSourceIds()))
	for _, s := range req.GetSourceIds() {
		id, err := parseID(s, "source id")
		if err != nil {
			return nil, err
		}

		sourceIDs = append(sourceIDs, id)
	}

	if err = h.ignoreService.Unignore(ctx, userID, sourceIDs); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) ListIgnoredSources(
	ctx context.Context,
	_ *userv1.ListIgnoredSourcesRequest,
) (*userv1.ListIgnoredSourcesResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ids, err := h.ignoreService.Ignored(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListIgnoredSourcesResponse{SourceIds: make([]string, len(ids))}
	for i, id := range ids {
		resp.SourceIds[i] = id.String()
	}

	return resp, nil
}
