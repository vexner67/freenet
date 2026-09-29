package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/logger"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/config"
	usergrpc "github.com/vexner67/freenet/user/internal/grpc"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

type App struct {
	logger *slog.Logger
	pool   *pgxpool.Pool
	server *grpc.Server
	health *health.Server
	addr   string
}

func New(cfg config.Config) (*App, error) {
	log, err := logger.New(logger.Config{
		LogLevel:  cfg.LogLevel,
		LogFormat: cfg.LogFormat,
	})
	if err != nil {
		return nil, errs.Wrap("logger.New", err)
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := postgres.New(dbCtx, cfg.DatabaseURL)
	if err != nil {
		return nil, errs.Wrap("postgres.New", err)
	}

	userRepo := inmem.NewUserRepository()
	userService := service.NewUserService(userRepo)
	userHandler := usergrpc.NewUserHandler(userService, log)

	server := grpc.NewServer()
	userv1.RegisterUserServiceServer(server, userHandler)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	reflection.Register(server)

	return &App{
		logger: log,
		pool:   pool,
		server: server,
		health: healthServer,
		addr:   fmt.Sprintf(":%d", cfg.GRPCPort),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", a.addr)
	if err != nil {
		return errs.Wrap("net.Listen", err)
	}
	defer func() {
		if err = lis.Close(); err != nil {
			a.logger.Error("listener close", "error", err)
		}
	}()

	a.health.SetServingStatus(
		"",
		grpc_health_v1.HealthCheckResponse_SERVING,
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- a.server.Serve(lis)
	}()

	a.logger.Info("user gRPC server started", "address", a.addr)

	select {
	case err = <-serveErr:
		if err != nil {
			return errs.Wrap("a.server.Serve", err)
		}
		return nil
	case <-ctx.Done():
		a.logger.Info("stopping user gRPC server")
	}

	a.health.Shutdown()

	timer := time.AfterFunc(5*time.Second, a.server.Stop)
	defer timer.Stop()

	a.server.GracefulStop()
	<-serveErr

	a.logger.Info("user gRPC server stopped")
	return nil
}

func (a *App) Close() {
	a.pool.Close()
}
