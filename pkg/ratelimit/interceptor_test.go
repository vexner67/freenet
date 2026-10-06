package ratelimit_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRules_UnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want string
		ok   bool
	}{
		{"5/1m,100/24h", "5/1m0s,100/24h0m0s", true},
		{" 30/1m ", "30/1m0s", true},
		{"", "", true},
		{"5", "", false},
		{"0/1m", "", false},
		{"five/1m", "", false},
		{"5/soon", "", false},
		{"5/-1m", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()

			var rules ratelimit.Rules
			err := rules.UnmarshalText([]byte(tt.text))
			if (err == nil) != tt.ok {
				t.Fatalf("UnmarshalText(%q) = %v, want ok = %v", tt.text, err, tt.ok)
			}
			if tt.ok && rules.String() != tt.want {
				t.Fatalf("rules = %s, want %s", rules, tt.want)
			}
		})
	}
}

type brokenLimiter struct{}

func (brokenLimiter) Allow(context.Context, string, int, time.Duration) (bool, error) {
	return false, errors.New("redis is down")
}

func TestUnaryServerInterceptor(t *testing.T) {
	t.Parallel()

	const (
		limited = "/post.v1.PostService/CreatePost"
		free    = "/post.v1.PostService/ListPosts"
	)

	interceptor := ratelimit.UnaryServerInterceptor(
		ratelimit.NewMemory(),
		map[string]ratelimit.Rules{limited: {{Limit: 2, Window: time.Minute}, {Limit: 3, Window: time.Hour}}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	ok := func(context.Context, any) (any, error) { return "done", nil }
	as := func(userID uuid.UUID) context.Context {
		return jwtauth.ContextWithIdentity(context.Background(), jwtauth.Identity{UserID: userID})
	}
	call := func(ctx context.Context, method string) codes.Code {
		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, ok)
		return status.Code(err)
	}

	ann, bob := uuid.New(), uuid.New()

	for i := 1; i <= 2; i++ {
		if got := call(as(ann), limited); got != codes.OK {
			t.Fatalf("call %d: %v", i, got)
		}
	}
	if got := call(as(ann), limited); got != codes.ResourceExhausted {
		t.Fatalf("third call in a minute: %v, want ResourceExhausted", got)
	}

	if got := call(as(bob), limited); got != codes.OK {
		t.Fatalf("bob has his own limit: %v", got)
	}
	for range 5 {
		if got := call(as(ann), free); got != codes.OK {
			t.Fatalf("a method without rules: %v", got)
		}
	}
	if got := call(context.Background(), limited); got != codes.OK {
		t.Fatalf("no token, not counted (jwtauth rejects it before): %v", got)
	}
}

func TestUnaryServerInterceptor_BrokenLimiter(t *testing.T) {
	t.Parallel()

	var logs strings.Builder
	interceptor := ratelimit.UnaryServerInterceptor(
		brokenLimiter{},
		map[string]ratelimit.Rules{"/m": {{Limit: 1, Window: time.Minute}}},
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	ctx := jwtauth.ContextWithIdentity(context.Background(), jwtauth.Identity{UserID: uuid.New()})
	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/m"}, func(context.Context, any) (any, error) { return "done", nil })

	if err != nil || resp != "done" {
		t.Fatalf("a broken limiter must let calls through: %v, %v", resp, err)
	}
	if !strings.Contains(logs.String(), "rate limit not checked") {
		t.Fatalf("the failure must be logged: %q", logs.String())
	}
}
