package grpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/mtls"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RoomPermissions interface {
	RoomAccess(context.Context, uuid.UUID, uuid.UUID) (service.RoomAccess, error)
}

// RoomAccessInterceptor opens this single RPC only to a verified room
// certificate. It does not add it to PublicMethods and also refuses plaintext.
func RoomAccessInterceptor(next grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	rule := map[string][]string{userv1.UserService_GetRoomAccess_FullMethodName: {mtls.Room}}
	internal := mtls.RestrictMethods(rule)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == userv1.UserService_GetRoomAccess_FullMethodName {
			return internal(ctx, req, info, handler)
		}
		return next(ctx, req, info, handler)
	}
}
func (h *UserHandler) GetRoomAccess(ctx context.Context, req *userv1.GetRoomAccessRequest) (*userv1.GetRoomAccessResponse, error) {
	owner, err := uuid.Parse(req.GetOwnerId())
	if err != nil || owner == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner id")
	}
	viewer, err := uuid.Parse(req.GetViewerId())
	if err != nil || viewer == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, "invalid viewer id")
	}
	access, err := h.roomPermissions.RoomAccess(ctx, owner, viewer)
	if err != nil {
		return nil, errs.Wrap("h.roomPermissions.RoomAccess", err)
	}
	return &userv1.GetRoomAccessResponse{Friend: access.Friend, Blocked: access.Blocked, Banned: access.Banned, CanInvite: access.CanInvite}, nil
}
