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
	// streams is the context of every stream; stopping cancels it, since
	// GracefulStop would otherwise wait for streams that never end.
	streams     context.Context
	stopStreams context.CancelFunc
}

func New(logger *slog.Logger, opts ...grpc.ServerOption) *Server {
	streams, stopStreams := context.WithCancel(context.Background())

	s := &Server{
		health:      health.NewServer(),
		logger:      logger,
		streams:     streams,
		stopStreams: stopStreams,
	}

	// Outermost, so the streams of every handler and interceptor end.
	s.grpc = grpc.NewServer(append([]grpc.ServerOption{grpc.ChainStreamInterceptor(s.endOnStop)}, opts...)...)

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
	s.stopStreams()

	timer := time.AfterFunc(stopTimeout, s.grpc.Stop)
	defer timer.Stop()

	s.grpc.GracefulStop()
	<-serveErr

	s.logger.Info("gRPC server stopped")

	return nil
}

// endOnStop cancels a stream's context when the server stops.
func (s *Server) endOnStop(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx, cancel := context.WithCancel(ss.Context())
	defer cancel()

	stop := context.AfterFunc(s.streams, cancel)
	defer stop()

	return handler(srv, &stream{ServerStream: ss, ctx: ctx})
}

type stream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stream) Context() context.Context {
	return s.ctx
}
