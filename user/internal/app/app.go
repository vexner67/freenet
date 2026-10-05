package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/config"
	usergrpc "github.com/vexner67/freenet/user/internal/grpc"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/notifyclient"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/migrations"
	"google.golang.org/grpc"
)

const (
	connectTimeout = 5 * time.Second
	migrateTimeout = time.Minute
)

type App struct {
	logger           *slog.Logger
	pool             *pgxpool.Pool
	notificationConn *grpc.ClientConn
	server           *grpcserver.Server
	addr             string
}

func New(cfg config.Config) (*App, error) {
	log, err := logger.New(logger.Config{
		LogLevel:  cfg.LogLevel,
		LogFormat: cfg.LogFormat,
	})
	if err != nil {
		return nil, errs.Wrap("logger.New", err)
	}

	a := &App{
		logger: log,
		addr:   fmt.Sprintf(":%d", cfg.GRPCPort),
	}

	// On failure Close releases whatever was opened before it.
	if err = a.build(cfg); err != nil {
		a.Close()
		return nil, errs.Wrap("a.build", err)
	}

	return a, nil
}

func (a *App) build(cfg config.Config) error {
	store, err := a.newStorage(cfg)
	if err != nil {
		return errs.Wrap("a.newStorage", err)
	}

	notifier, err := a.newNotifier(cfg)
	if err != nil {
		return errs.Wrap("a.newNotifier", err)
	}

	publicKey, err := jwtauth.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return errs.Wrap("jwtauth.LoadPublicKey", err)
	}

	creds, err := mtls.ServerCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ServerCredentials", err)
	}

	interceptors := []grpc.UnaryServerInterceptor{usergrpc.ErrorInterceptor(a.logger)}

	if cfg.TLS.Enabled() {
		interceptors = append(interceptors, mtls.RestrictMethods(usergrpc.InternalMethods()))
	} else {
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	interceptors = append(interceptors, jwtauth.UnaryServerInterceptor(
		jwtauth.NewVerifier(publicKey, cfg.JWTIssuer),
		usergrpc.PublicMethods()...,
	))

	a.server = grpcserver.New(a.logger, grpc.Creds(creds), grpc.ChainUnaryInterceptor(interceptors...))
	userv1.RegisterUserServiceServer(
		a.server.Registrar(),
		usergrpc.NewUserHandler(
			service.NewUserService(store.users),
			service.NewFollowService(store.users, store.follows, store.follows, notifier),
			a.logger,
		),
	)

	a.logger.Info("user configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(),
		"notifications", cfg.NotificationServiceAddr != "")

	return nil
}

// newNotifier connects to the notification service; the connection is
// lazy, so the services may start in any order.
func (a *App) newNotifier(cfg config.Config) (service.Notifier, error) {
	if cfg.NotificationServiceAddr == "" {
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: follows notify nobody")
		return notifyclient.Nop{}, nil
	}

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	a.notificationConn, err = grpc.NewClient(cfg.NotificationServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, errs.Wrap("grpc.NewClient", err)
	}

	return notifyclient.New(notificationv1.NewNotificationServiceClient(a.notificationConn), cfg.NotifyTimeout, a.logger), nil
}

// storage is the set of repositories chosen by STORAGE.
type storage struct {
	users   service.UserRepository
	follows interface {
		service.FollowRepository
		service.FollowListRepository
	}
}

func (a *App) newStorage(cfg config.Config) (storage, error) {
	if cfg.Storage == config.StorageInmem {
		users := inmem.NewUserRepository()
		return storage{users: users, follows: inmem.NewFollowRepository(users)}, nil
	}

	if cfg.MigrateOnStart {
		ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
		defer cancel()

		applied, err := pgpool.Migrate(ctx, cfg.DatabaseURL, migrations.FS)
		if err != nil {
			return storage{}, errs.Wrap("pgpool.Migrate", err)
		}

		a.logger.Info("migrations applied", "count", applied)
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	var err error

	a.pool, err = pgpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return storage{}, errs.Wrap("pgpool.New", err)
	}

	return storage{
		users:   postgres.NewUserRepository(a.pool),
		follows: postgres.NewFollowRepository(a.pool),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.server.Run(ctx, a.addr); err != nil {
		return errs.Wrap("a.server.Run", err)
	}

	return nil
}

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	if a.notificationConn != nil {
		if err := a.notificationConn.Close(); err != nil {
			a.logger.Error("notification connection close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
