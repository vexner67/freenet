package grpc

import (
	"github.com/vexner67/freenet/pkg/mtls"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		postv1.PostService_PurgeUser_FullMethodName: {mtls.User},
	}
}

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). The internal methods
// carry no user token: other services call them, checked by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		postv1.PostService_GetPost_FullMethodName,
		postv1.PostService_GetPosts_FullMethodName,
		postv1.PostService_ListPosts_FullMethodName,
		postv1.PostService_ListFeed_FullMethodName,
		postv1.PostService_ListComments_FullMethodName,
		postv1.PostService_GetComments_FullMethodName,
		postv1.PostService_PurgeUser_FullMethodName,
	}
}
