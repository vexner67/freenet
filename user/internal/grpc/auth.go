package grpc

import (
	"github.com/vexner67/freenet/pkg/mtls"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor).
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		userv1.UserService_GetProfile_FullMethodName,
		userv1.UserService_GetProfiles_FullMethodName,
		userv1.UserService_SearchUsers_FullMethodName,
		userv1.UserService_GetFollowStats_FullMethodName,
		userv1.UserService_ListFollowers_FullMethodName,
		userv1.UserService_ListFollowing_FullMethodName,
		userv1.UserService_GetOrCreateByEmail_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		userv1.UserService_GetOrCreateByEmail_FullMethodName: {mtls.Auth},
	}
}
