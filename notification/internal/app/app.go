package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/notification/internal/config"
	notificationgrpc "github.com/vexner67/freenet/notification/internal/grpc"
	"github.com/vexner67/freenet/notification/internal/inmem"
	"github.com/vexner67/freenet/notification/internal/postgres"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/notification/migrations"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/pubsub"
	"google.golang.org/grpc"
)

const (
	connectTimeout = 5 * time.Second
	migrateTimeout = time.Minute
	// eventBuffer is how far a subscriber may lag before it is dropped
	// and has to catch up from storage.
	eventBuffer = 64
)

type App struct {
	logger *slog.Logger
	pool   *pgxpool.Pool
	server *grpcserver.Server
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
	repos, err := a.newRepositories(cfg)
	if err != nil {
		return errs.Wrap("a.newRepositories", err)
	}

	publicKey, err := jwtauth.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return errs.Wrap("jwtauth.LoadPublicKey", err)
	}

	creds, err := mtls.ServerCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ServerCredentials", err)
	}

	interceptors := []grpc.UnaryServerInterceptor{notificationgrpc.ErrorInterceptor(a.logger)}

	if cfg.TLS.Enabled() {
		interceptors = append(interceptors, mtls.RestrictMethods(notificationgrpc.InternalMethods()))
	} else {
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	verifier := jwtauth.NewVerifier(publicKey, cfg.JWTIssuer)
	interceptors = append(interceptors, jwtauth.UnaryServerInterceptor(verifier, notificationgrpc.PublicMethods()...))

	a.server = grpcserver.New(
		a.logger,
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(interceptors...),
		grpc.ChainStreamInterceptor(jwtauth.StreamServerInterceptor(verifier)),
	)

	// Live updates stay within this replica; see Subscribe in the proto.
	events := pubsub.New[uuid.UUID, service.Change](eventBuffer)

	notificationv1.RegisterNotificationServiceServer(
		a.server.Registrar(),
		notificationgrpc.NewNotificationHandler(
			service.NewNotificationService(repos.notifications, repos.readMarks, events),
			a.logger,
		),
	)

	a.logger.Info("notification configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled())

	return nil
}

type repositories struct {
	notifications service.NotificationRepository
	readMarks     service.ReadMarkRepository
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		return repositories{
			notifications: inmem.NewNotificationRepository(),
			readMarks:     inmem.NewReadMarkRepository(),
		}, nil
	}

	if cfg.MigrateOnStart {
		ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
		defer cancel()

		applied, err := pgpool.Migrate(ctx, cfg.DatabaseURL, migrations.FS)
		if err != nil {
			return repositories{}, errs.Wrap("pgpool.Migrate", err)
		}

		a.logger.Info("migrations applied", "count", applied)
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	var err error

	a.pool, err = pgpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return repositories{}, errs.Wrap("pgpool.New", err)
	}

	return repositories{
		notifications: postgres.NewNotificationRepository(a.pool),
		readMarks:     postgres.NewReadMarkRepository(a.pool),
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
	if a.pool != nil {
		a.pool.Close()
	}
}
