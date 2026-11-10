package grpc

import (
	"context"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/mtls"
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
	"google.golang.org/grpc"
)

func PublicMethods() []string {
	return []string{
		roomv1.RoomService_GetRoom_FullMethodName,
		roomv1.RoomService_ListUserRooms_FullMethodName,
		roomv1.RoomService_ListClubRooms_FullMethodName,
	}
}
func InternalMethods() map[string][]string {
	return map[string][]string{
		roomv1.RoomService_AuthorizeMedia_FullMethodName: {mtls.Gateway},
		roomv1.RoomService_CurrentRooms_FullMethodName:   {mtls.Gateway},
		roomv1.RoomService_PurgeUser_FullMethodName:      {mtls.User},
		roomv1.RoomService_PurgeClub_FullMethodName:      {mtls.Club},
	}
}

// AuthInterceptor always checks verified mTLS identities for internal RPCs,
// including when the rest of the development server uses plaintext. No
// cleanup RPC is public. An optional JWT on CurrentRooms still identifies
// its viewer and is verified only after the service identity is accepted.
func AuthInterceptor(verifier *jwtauth.Verifier) grpc.UnaryServerInterceptor {
	rules := InternalMethods()
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	restricted := mtls.RestrictMethods(rules)
	userAuth := jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: PublicMethods()})
	internalViewer := jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: names})
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, internal := rules[info.FullMethod]; internal {
			return restricted(ctx, req, info, func(ctx context.Context, req any) (any, error) { return internalViewer(ctx, req, info, handler) })
		}
		return userAuth(ctx, req, info, handler)
	}
}
