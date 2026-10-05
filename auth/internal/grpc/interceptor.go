package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns auth errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	case errors.Is(err, service.ErrInvalidCode),
		errors.Is(err, service.ErrCodeNotFound):
		// One answer for both cases, so it is impossible to tell
		// whether a code was ever requested for the email.
		return status.New(codes.Unauthenticated, "invalid or expired code"), true
	case errors.Is(err, service.ErrInvalidRefreshToken),
		errors.Is(err, service.ErrSessionNotFound):
		return status.New(codes.Unauthenticated, "invalid refresh token"), true
	case errors.Is(err, service.ErrTooManyAttempts):
		return status.New(codes.ResourceExhausted, "too many attempts, request a new code"), true
	case errors.Is(err, service.ErrTooManyRequests):
		return status.New(codes.ResourceExhausted, "too many requests, try again later"), true
	case errors.Is(err, account.ErrInvalidEmail):
		return status.New(codes.InvalidArgument, "invalid email"), true
	case errors.Is(err, service.ErrUserServiceUnavailable):
		return status.New(codes.Unavailable, "service temporarily unavailable"), true
	default:
		return nil, false
	}
}
