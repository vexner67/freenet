package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/auth/internal/config"
	"github.com/vexner67/freenet/auth/internal/console"
	"github.com/vexner67/freenet/auth/internal/errs"
	authgrpc "github.com/vexner67/freenet/auth/internal/grpc"
	"github.com/vexner67/freenet/auth/internal/hmac"
	"github.com/vexner67/freenet/auth/internal/inmem"
	"github.com/vexner67/freenet/auth/internal/logger"
	"github.com/vexner67/freenet/auth/internal/postgres"
	"github.com/vexner67/freenet/auth/internal/service"
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
	log, err := logger.New(cfg)
	if err != nil {
		return nil, errs.Wrap("logger.New", err)
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := postgres.New(dbCtx, cfg.DatabaseURL)
	if err != nil {
		return nil, errs.Wrap("postgres.New", err)
	}

	sessionRepo := inmem.NewSessionRepository()
	hasher := hmac.NewHasher([]byte(cfg.HashSecret))
	codeRepo := inmem.NewCodeRepository()
	consoleMailer := console.NewMailer(log)
	authService := service.NewAuthService(sessionRepo, hasher, codeRepo, consoleMailer)
	authHandler := authgrpc.NewAuthHandler(authService, log)

	server := grpc.NewServer()
	authv1.RegisterAuthServiceServer(server, authHandler)

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

	a.logger.Info("gRPC server started", "address", a.addr)

	select {
	case err = <-serveErr:
		if err != nil {
			return errs.Wrap("a.server.Serve", err)
		}
		return nil
	case <-ctx.Done():
		a.logger.Info("stopping gRPC server")
	}

	a.health.Shutdown()

	timer := time.AfterFunc(5*time.Second, a.server.Stop)
	defer timer.Stop()

	a.server.GracefulStop()
	<-serveErr

	a.logger.Info("gRPC server stopped")
	return nil
}

func (a *App) Close() {
	a.pool.Close()
}
