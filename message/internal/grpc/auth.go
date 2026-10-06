package grpc

import (
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). Messages are private:
// only health checks are public.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
	}
}
