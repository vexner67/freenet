package grpc

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"google.golang.org/grpc/codes"
)

func TestToStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		known   bool
		code    codes.Code
		message string
	}{
		{"wrong code", errs.Wrap("s.verifyCode", service.ErrInvalidCode), true, codes.Unauthenticated, "invalid or expired code"},
		{"no code looks the same as a wrong one", errs.Wrap("s.codeRepo.Get", service.ErrCodeNotFound), true, codes.Unauthenticated, "invalid or expired code"},
		{"invalid refresh token", errs.Wrap("s.currentSession", service.ErrInvalidRefreshToken), true, codes.Unauthenticated, "invalid refresh token"},
		{"session is gone", errs.Wrap("s.sessionRepo.Get", service.ErrSessionNotFound), true, codes.Unauthenticated, "invalid refresh token"},
		{"out of attempts", errs.Wrap("s.verifyCode", service.ErrTooManyAttempts), true, codes.ResourceExhausted, "too many attempts, request a new code"},
		{"rate limited", errs.Wrap("s.allow", service.ErrTooManyRequests), true, codes.ResourceExhausted, "too many requests, try again later"},
		{"invalid email", errs.Wrap("c.api.GetOrCreateByEmail", account.ErrInvalidEmail), true, codes.InvalidArgument, "invalid email"},
		{"user service down", errs.Wrap("s.userProvider.GetOrCreateByEmail", service.ErrUserServiceUnavailable), true, codes.Unavailable, "service temporarily unavailable"},
		{"unknown", errors.New("dial tcp 10.0.0.5:6379: connection refused"), false, codes.OK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st, known := toStatus(tt.err)
			if known != tt.known {
				t.Fatalf("known = %v, want %v", known, tt.known)
			}
			if known && (st.Code() != tt.code || st.Message() != tt.message) {
				t.Fatalf("got %v %q, want %v %q", st.Code(), st.Message(), tt.code, tt.message)
			}
		})
	}
}
