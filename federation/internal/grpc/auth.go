package grpc

import (
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). The internal methods
// carry no user token: other services call them, checked by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		federationv1.FederationService_Finger_FullMethodName,
		federationv1.FederationService_GetActor_FullMethodName,
		federationv1.FederationService_GetNote_FullMethodName,
		federationv1.FederationService_GetFollowers_FullMethodName,
		federationv1.FederationService_GetOutbox_FullMethodName,
		federationv1.FederationService_Receive_FullMethodName,
		federationv1.FederationService_PostChanged_FullMethodName,
	}
}

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		federationv1.FederationService_Receive_FullMethodName:     {mtls.Gateway},
		federationv1.FederationService_PostChanged_FullMethodName: {mtls.Post},
	}
}
