package jwtauth

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
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

// ErrPageDeleted refuses the token of a deleted page (Claims.Deactivated)
// on what only an active page may do.
var ErrPageDeleted = errors.New("your page is deleted")

// Access lists the methods (full names, e.g.
// "/user.v1.UserService/GetProfile") that need less than the access token
// of an active page.
type Access struct {
	// Public methods work without a token.
	Public []string
	// Deactivated methods take the token of a deleted page too: its owner
	// sees and restores it with them.
	Deactivated []string
}

// UnaryServerInterceptor requires a valid access token in the
// "authorization: Bearer <token>" metadata on every call except the
// public ones. Secure by default: a new RPC is protected unless listed
// explicitly.
//
// Public methods work without a token, but a token that is sent is still
// verified: then the handler knows the viewer (e.g. "you follow them"),
// and an expired token is reported so the client can refresh it.
//
// The token of a deleted page works on the Deactivated methods; public
// ones see its owner as an anonymous viewer, and all others refuse it
// with PermissionDenied (ErrPageDeleted).
//
// The health check is always public: it tells only whether the service
// serves, and the container healthcheck has no token.
func UnaryServerInterceptor(v *Verifier, access Access) grpc.UnaryServerInterceptor {
	public := set(access.Public)
	public[grpc_health_v1.Health_Check_FullMethodName] = struct{}{}
	deactivated := set(access.Deactivated)

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

		if id.Deactivated {
			if _, ok := deactivated[info.FullMethod]; !ok {
				if isPublic {
					return handler(ctx, req)
				}

				return nil, status.Error(codes.PermissionDenied, ErrPageDeleted.Error())
			}
		}

		return handler(ContextWithIdentity(ctx, id), req)
	}
}

func set(methods []string) map[string]struct{} {
	s := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		s[m] = struct{}{}
	}

	return s
}

// StreamServerInterceptor requires a valid access token on every stream:
// streams carry a user's own events, none is public. The stream lives no
// longer than the token: at its expiry the context's deadline ends it, and
// the handler reports Unauthenticated (see ErrTokenExpired), so the client
// subscribes again with a fresh token. A token that was revoked meanwhile
// keeps its stream until then, as it keeps its calls. The token of a
// deleted page gets no streams.
func StreamServerInterceptor(v *Verifier) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		token, ok := bearerToken(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing access token")
		}

		id, err := v.Verify(token, AccessToken)
		if err != nil {
			return status.Error(codes.Unauthenticated, "invalid access token")
		}

		if id.Deactivated {
			return status.Error(codes.PermissionDenied, ErrPageDeleted.Error())
		}

		ctx, cancel := context.WithDeadline(ContextWithIdentity(ss.Context(), id), id.ExpiresAt)
		defer cancel()

		return handler(srv, &identityStream{ServerStream: ss, ctx: ctx})
	}
}

// StreamEnd is what a stream handler returns when its context is done: the
// token expired (Unauthenticated, to subscribe again with a fresh one) or
// the client left or the server stops (Canceled).
func StreamEnd(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return status.Error(codes.Unauthenticated, ErrTokenExpired.Error())
	}

	return status.Error(codes.Canceled, "stream closed")
}

// ErrTokenExpired ends a stream whose access token expired.
var ErrTokenExpired = errors.New("access token expired")

type identityStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *identityStream) Context() context.Context {
	return s.ctx
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
