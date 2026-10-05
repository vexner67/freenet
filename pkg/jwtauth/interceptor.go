package jwtauth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type identityKey struct{}

func ContextWithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// UnaryServerInterceptor requires a valid access token in the
// "authorization: Bearer <token>" metadata on every call except
// publicMethods (full names, e.g. "/user.v1.UserService/GetProfile").
// Secure by default: a new RPC is protected unless listed explicitly.
//
// Public methods work without a token, but a token that is sent is still
// verified: then the handler knows the viewer (e.g. "you follow them"),
// and an expired token is reported so the client can refresh it.
func UnaryServerInterceptor(v *Verifier, publicMethods ...string) grpc.UnaryServerInterceptor {
	public := make(map[string]struct{}, len(publicMethods))
	for _, m := range publicMethods {
		public[m] = struct{}{}
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		_, isPublic := public[info.FullMethod]

		token, ok := bearerToken(ctx)
		if !ok {
			if isPublic {
				return handler(ctx, req)
			}

			return nil, status.Error(codes.Unauthenticated, "missing access token")
		}

		id, err := v.Verify(token, AccessToken)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid access token")
		}

		return handler(ContextWithIdentity(ctx, id), req)
	}
}

func bearerToken(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}

	values := md.Get("authorization")
	if len(values) != 1 {
		return "", false
	}

	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}

	return token, true
}
