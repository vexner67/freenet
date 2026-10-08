package grpc

import (
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). The internal methods
// carry no user token: other services call them, checked by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		mediav1.MediaService_GetAvatar_FullMethodName,
		mediav1.MediaService_GetPhoto_FullMethodName,
		mediav1.MediaService_PublishPhotos_FullMethodName,
		mediav1.MediaService_DeletePublishedPhotos_FullMethodName,
		mediav1.MediaService_PurgeUser_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		mediav1.MediaService_PublishPhotos_FullMethodName:         {mtls.Post, mtls.Message},
		mediav1.MediaService_DeletePublishedPhotos_FullMethodName: {mtls.Post, mtls.Message},
		mediav1.MediaService_PurgeUser_FullMethodName:             {mtls.User},
	}
}
