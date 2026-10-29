package grpc

import (
	"github.com/vexner67/freenet/pkg/mtls"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). The world is open to
// guests; the internal methods carry no user token: the gateway calls them,
// checked by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		worldv1.WorldService_GetWorld_FullMethodName,
		worldv1.WorldService_GetAtlas_FullMethodName,
		worldv1.WorldService_GetRaster_FullMethodName,
		worldv1.WorldService_GetTile_FullMethodName,
		worldv1.WorldService_ListWorlds_FullMethodName,
		worldv1.WorldService_CreateWorld_FullMethodName,
		worldv1.WorldService_OpenWorld_FullMethodName,
		worldv1.WorldService_EndWorld_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		worldv1.WorldService_ListWorlds_FullMethodName:  {mtls.Gateway},
		worldv1.WorldService_CreateWorld_FullMethodName: {mtls.Gateway},
		worldv1.WorldService_OpenWorld_FullMethodName:   {mtls.Gateway},
		worldv1.WorldService_EndWorld_FullMethodName:    {mtls.Gateway},
	}
}
