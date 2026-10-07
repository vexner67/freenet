package grpcserver

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Probe asks the server at addr whether it is serving. It is the container
// healthcheck: the images have no shell or grpc_health_probe.
func Probe(ctx context.Context, addr string, creds credentials.TransportCredentials) error {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return fmt.Errorf("grpc.NewClient: %w", err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("client.Check: %w", err)
	}

	if s := resp.GetStatus(); s != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("status %s", s)
	}

	return nil
}
