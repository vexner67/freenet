package grpcserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// freeAddr is a local address nobody listens on right now.
func freeAddr(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	return addr
}

// An open stream, here the endless health Watch, must not hold the server
// up until the graceful stop times out.
func TestServer_StopEndsStreams(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	srv := grpcserver.New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx, addr) }()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var watch grpc_health_v1.Health_WatchClient
	for range 50 {
		watch, err = grpc_health_v1.NewHealthClient(conn).Watch(context.Background(), &grpc_health_v1.HealthCheckRequest{})
		if err == nil {
			if _, err = watch.Recv(); err == nil {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	started := time.Now()
	stop()

	if err = <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if took := time.Since(started); took > 2*time.Second {
		t.Fatalf("stopping took %v: the stream held it up", took)
	}

	for {
		if _, err = watch.Recv(); err != nil {
			break // the stream ended
		}
	}
}
