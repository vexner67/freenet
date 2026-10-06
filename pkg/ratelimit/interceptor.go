package ratelimit

import (
	"context"
	"log/slog"
	"time"

	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Limiter is Memory or Redis.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// UnaryServerInterceptor limits how often each user calls the methods in
// rules, keyed by method; an exceeded rule is ResourceExhausted.
//
// It goes after jwtauth.UnaryServerInterceptor: the user comes from the
// verified token, never from the request. Calls without a token are not
// counted; the limited methods require one anyway. A limiter that fails,
// Redis being down, lets calls through: spam protection must not stop
// people from posting. The failure is logged.
func UnaryServerInterceptor(limiter Limiter, rules map[string]Rules, log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		limits := rules[info.FullMethod]
		identity, signedIn := jwtauth.IdentityFromContext(ctx)

		if len(limits) == 0 || !signedIn {
			return handler(ctx, req)
		}

		for _, rule := range limits {
			key := info.FullMethod + ":" + rule.Window.String() + ":" + identity.UserID.String()

			allowed, err := limiter.Allow(ctx, key, rule.Limit, rule.Window)
			if err != nil {
				log.WarnContext(ctx, "rate limit not checked", "method", info.FullMethod, "error", err)
				return handler(ctx, req)
			}

			if !allowed {
				return nil, status.Error(codes.ResourceExhausted, "too many requests, try again later")
			}
		}

		return handler(ctx, req)
	}
}
