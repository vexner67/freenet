package grpc

import (
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor).
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		mediav1.MediaService_GetAvatar_FullMethodName,
		mediav1.MediaService_GetPhoto_FullMethodName,
	}
}
