package grpcserver_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errKnown = errors.New("known")

func translate(err error) (*status.Status, bool) {
	if errors.Is(err, errKnown) {
		return status.New(codes.NotFound, "thing not found"), true
	}
	return nil, false
}

func TestErrorInterceptor(t *testing.T) {
	t.Parallel()

	interceptor := grpcserver.ErrorInterceptor(slog.New(slog.NewTextHandler(io.Discard, nil)), translate)

	tests := []struct {
		name    string
		err     error
		code    codes.Code
		message string
	}{
		{"known error, wrapped", fmt.Errorf("repo.Get: %w", errKnown), codes.NotFound, "thing not found"},
		{"status from handler passes", status.Error(codes.InvalidArgument, "bad nickname"), codes.InvalidArgument, "bad nickname"},
		{"unknown error does not leak", errors.New("dial tcp 10.0.0.5:5432: refused"), codes.Internal, "internal error"},
		{"wrapped downstream status does not leak", fmt.Errorf("client.Call: %w", status.Error(codes.PermissionDenied, "secret")), codes.Internal, "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
				func(context.Context, any) (any, error) { return nil, tt.err })

			got := status.Convert(err)
			if got.Code() != tt.code || got.Message() != tt.message {
				t.Fatalf("got %v %q, want %v %q", got.Code(), got.Message(), tt.code, tt.message)
			}
		})
	}

	t.Run("success passes the response", func(t *testing.T) {
		t.Parallel()

		resp, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
			func(context.Context, any) (any, error) { return "ok", nil })
		if err != nil || resp != "ok" {
			t.Fatalf("got %v, %v", resp, err)
		}
	})
}
