package grpcclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// A call made while the service is down waits for it to come back.
func TestNew_WaitsForTheService(t *testing.T) {
	t.Parallel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := lis.Addr().String()
	_ = lis.Close()

	conn, err := grpcclient.New(addr, insecure.NewCredentials())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Nobody listens: the connection fails first.
	conn.Connect()
	for state := conn.GetState(); state != connectivity.TransientFailure; state = conn.GetState() {
		if !conn.WaitForStateChange(ctx, state) {
			t.Fatalf("the connection is %s, want it to fail", state)
		}
	}

	lis, err = net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}

	srv := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	if _, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatalf("Check: %v", err)
	}
}
