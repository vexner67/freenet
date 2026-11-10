package grpc

import (
	"context"
	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The explicit user ID is accepted only from a verified room certificate,
// including in local plaintext mode. Existing JWT policies stay unchanged.
func RoomAccessInterceptor(next grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	internal := mtls.RestrictMethods(map[string][]string{clubv1.ClubService_GetRoomAccess_FullMethodName: {mtls.Room}})
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == clubv1.ClubService_GetRoomAccess_FullMethodName {
			return internal(ctx, req, info, handler)
		}
		return next(ctx, req, info, handler)
	}
}
func (h *ClubHandler) GetRoomAccess(ctx context.Context, req *clubv1.GetRoomAccessRequest) (*clubv1.GetRoomAccessResponse, error) {
	clubID, err := uuid.Parse(req.GetClubId())
	if err != nil || clubID == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, "invalid club id")
	}
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil || userID == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}
	view, err := h.clubService.Find(ctx, userID, service.Ref{ID: clubID})
	if err != nil {
		return nil, errs.Wrap("h.clubService.Find", err)
	}
	return &clubv1.GetRoomAccessResponse{Member: view.Role != club.RoleNone, Manager: view.Role.Manages()}, nil
}
