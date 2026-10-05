// Package grpcserver runs the gRPC servers of freenet services the same way:
// health checks, reflection, graceful shutdown and safe error statuses.
package grpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// stopTimeout bounds GracefulStop; after it in-flight calls are cut.
const stopTimeout = 5 * time.Second

type Server struct {
	grpc   *grpc.Server
	health *health.Server
	logger *slog.Logger
}

func New(logger *slog.Logger, opts ...grpc.ServerOption) *Server {
	s := &Server{
		grpc:   grpc.NewServer(opts...),
		health: health.NewServer(),
		logger: logger,
	}

	grpc_health_v1.RegisterHealthServer(s.grpc, s.health)
	reflection.Register(s.grpc)

	return s
}

// Registrar is what generated Register<Service>Server functions take.
func (s *Server) Registrar() grpc.ServiceRegistrar {
	return s.grpc
}

// Run serves on addr until ctx is cancelled, then stops gracefully.
func (s *Server) Run(ctx context.Context, addr string) error {
	// Serve takes ownership of lis and closes it on return or Stop.
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("net.Listen: %w", err)
	}

	s.health.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.grpc.Serve(lis)
	}()

	s.logger.Info("gRPC server started", "address", addr)

	select {
	case err = <-serveErr:
		if err != nil {
			return fmt.Errorf("s.grpc.Serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		s.logger.Info("stopping gRPC server")
	}

	s.health.Shutdown()

	timer := time.AfterFunc(stopTimeout, s.grpc.Stop)
	defer timer.Stop()

	s.grpc.GracefulStop()
	<-serveErr

	s.logger.Info("gRPC server stopped")

	return nil
}
