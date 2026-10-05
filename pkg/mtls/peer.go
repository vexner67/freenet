package mtls

import (
	"context"
	"slices"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// PeerIdentity is the common name of the caller's verified certificate.
// It is empty without mTLS.
func PeerIdentity(ctx context.Context) (string, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", false
	}

	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.VerifiedChains) == 0 || len(info.State.VerifiedChains[0]) == 0 {
		return "", false
	}

	name := info.State.VerifiedChains[0][0].Subject.CommonName

	return name, name != ""
}

// RestrictMethods allows the listed full method names only to the given
// service identities, e.g. an internal RPC only to the auth service.
// Methods not listed are not restricted. Requires mTLS: without a verified
// certificate every restricted call is denied.
func RestrictMethods(rules map[string][]string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		allowed, restricted := rules[info.FullMethod]
		if !restricted {
			return handler(ctx, req)
		}

		identity, ok := PeerIdentity(ctx)
		if !ok || !slices.Contains(allowed, identity) {
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}

		return handler(ctx, req)
	}
}
