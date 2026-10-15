package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/notification/internal/config"
	notificationgrpc "github.com/vexner67/freenet/notification/internal/grpc"
	"github.com/vexner67/freenet/notification/internal/inmem"
	"github.com/vexner67/freenet/notification/internal/peopleclient"
	"github.com/vexner67/freenet/notification/internal/postgres"
	notificationredis "github.com/vexner67/freenet/notification/internal/redis"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/notification/internal/webpush"
	"github.com/vexner67/freenet/notification/migrations"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcclient"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/pubsub"
	"github.com/vexner67/freenet/pkg/tracing"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// probeTimeout bounds the healthcheck.
	probeTimeout   = 3 * time.Second
	connectTimeout = 5 * time.Second
	migrateTimeout = time.Minute
	// eventBuffer is how far a subscriber may lag before it is dropped
	// and has to catch up from storage.
	eventBuffer = 64
)

type App struct {
	logger *slog.Logger
	tracer *tracing.Tracer
	pool   *pgxpool.Pool
	redis  *goredis.Client
	// shared carries live events between replicas; nil keeps them here.
	shared  *pubsub.Redis[uuid.UUID, service.Change]
	server  *grpcserver.Server
	metrics *metrics.Server
	addr    string
	// cleanup forgets the notifications older than the retention.
	cleanup      *service.CleanupService
	cleanupEvery time.Duration
	// pushes tell browsers what happens, pushWorkers at once.
	pushes      *service.PushService
	pushWorkers int
	peopleConns []*grpc.ClientConn
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
	tracer, err := tracing.New(context.Background(), "notification", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer

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
	interceptors = append(interceptors, jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: notificationgrpc.PublicMethods()}))

	reg := metrics.NewRegistry()

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		a.tracer.ServerOption(),
		grpc.ChainUnaryInterceptor(interceptors...),
		grpc.ChainStreamInterceptor(jwtauth.StreamServerInterceptor(verifier)),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}

	a.pushes, err = a.newPushes(cfg, repos.subscriptions)
	if err != nil {
		return errs.Wrap("a.newPushes", err)
	}
	a.pushWorkers = cfg.PushWorkers

	notificationv1.RegisterNotificationServiceServer(
		a.server.Registrar(),
		notificationgrpc.NewNotificationHandler(
			service.NewNotificationService(repos.notifications, repos.readMarks, a.newEvents(cfg), a.pushes),
			a.pushes,
			a.logger,
		),
	)

	a.cleanup = service.NewCleanupService(repos.old, cfg.Retention)
	a.cleanupEvery = cfg.CleanupInterval

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("notification configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(), "push", cfg.PushKeyPath != "")

	return nil
}

type repositories struct {
	notifications service.NotificationRepository
	readMarks     service.ReadMarkRepository
	old           service.OldNotificationDeleter
	subscriptions service.SubscriptionRepository
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		notifications := inmem.NewNotificationRepository()

		return repositories{
			notifications: notifications,
			readMarks:     inmem.NewReadMarkRepository(),
			old:           notifications,
			subscriptions: inmem.NewSubscriptionRepository(),
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

	notifications := postgres.NewNotificationRepository(a.pool)

	return repositories{
		notifications: notifications,
		readMarks:     postgres.NewReadMarkRepository(a.pool),
		old:           notifications,
		subscriptions: postgres.NewSubscriptionRepository(a.pool),
	}, nil
}

// Run serves and shares live events until ctx is cancelled or the gRPC or
// metrics server fails, which stops the rest too.
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return errs.Wrap("a.server.Run", a.server.Run(ctx, a.addr))
	})

	if a.metrics != nil {
		g.Go(func() error {
			return errs.Wrap("a.metrics.Run", a.metrics.Run(ctx))
		})
	}

	if a.shared != nil {
		g.Go(func() error {
			return errs.Wrap("a.shared.Run", a.shared.Run(ctx))
		})
	}

	g.Go(func() error {
		a.runCleanup(ctx)
		return nil
	})

	g.Go(func() error {
		a.pushes.Run(ctx, a.pushWorkers)
		return nil
	})

	return g.Wait()
}

// runCleanup deletes the old notifications at the start and then every
// interval until ctx ends; a failure is logged and tried again next time.
func (a *App) runCleanup(ctx context.Context) {
	t := time.NewTicker(a.cleanupEvery)
	defer t.Stop()

	for {
		deleted, err := a.cleanup.DeleteOld(ctx)
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "cleanup of old notifications", "error", err)
		}
		if deleted > 0 {
			a.logger.InfoContext(ctx, "old notifications deleted", "count", deleted)
		}

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Healthcheck asks the service running with cfg on this host whether it
// serves: the container healthcheck, as the image has no shell or curl.
func Healthcheck(ctx context.Context, cfg config.Config) error {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ClientCredentials", err)
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return errs.Wrap("grpcserver.Probe", grpcserver.Probe(ctx, fmt.Sprintf("localhost:%d", cfg.GRPCPort), creds))
}

// newEvents shares live events between the replicas through Redis when it
// is configured; otherwise a stream gets only the events of its replica.
func (a *App) newEvents(cfg config.Config) service.Events {
	if cfg.RedisAddr == "" {
		a.logger.Warn("REDIS_ADDR is not set: live events stay within this replica")
		return pubsub.New[uuid.UUID, service.Change](eventBuffer)
	}

	a.redis = goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	a.shared = pubsub.NewRedis(a.redis, notificationredis.Channel, notificationredis.ChangeCodec{}, eventBuffer, a.logger)

	return a.shared
}

// newPushes pushes with the VAPID key of cfg; without one pushes are off.
// The user and club services name who did it.
func (a *App) newPushes(cfg config.Config, subscriptions service.SubscriptionRepository) (*service.PushService, error) {
	deps := service.PushDeps{
		Subscriptions: subscriptions,
		Actors:        peopleclient.Nop{},
		Logger:        a.logger,
		Queue:         cfg.PushQueue,
		Timeout:       cfg.PushTimeout,
	}

	if cfg.PushKeyPath == "" {
		a.logger.Warn("PUSH_KEY_PATH is not set: browsers get no push notifications")
		return service.NewPushService(deps), nil
	}

	sender, err := webpush.Load(cfg.PushKeyPath, cfg.PushSubject, cfg.PushTimeout)
	if err != nil {
		return nil, errs.Wrap("webpush.Load", err)
	}
	deps.Sender = sender

	if cfg.UserServiceAddr == "" {
		a.logger.Warn("USER_SERVICE_ADDR is not set: pushes name nobody")
		return service.NewPushService(deps), nil
	}

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	users, err := grpcclient.New(cfg.UserServiceAddr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, errs.Wrap("grpcclient.New", err)
	}
	a.peopleConns = append(a.peopleConns, users)

	var clubs clubv1.ClubServiceClient

	if cfg.ClubServiceAddr == "" {
		a.logger.Warn("CLUB_SERVICE_ADDR is not set: pushes do not name communities")
	} else {
		conn, err := grpcclient.New(cfg.ClubServiceAddr, creds, a.tracer.DialOption())
		if err != nil {
			return nil, errs.Wrap("grpcclient.New", err)
		}
		a.peopleConns = append(a.peopleConns, conn)
		clubs = clubv1.NewClubServiceClient(conn)
	}

	deps.Actors = peopleclient.New(userv1.NewUserServiceClient(users), clubs, cfg.PeopleTimeout)

	return service.NewPushService(deps), nil
}

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	for _, conn := range a.peopleConns {
		if err := conn.Close(); err != nil {
			a.logger.Error("grpc client close", "error", err)
		}
	}

	if a.tracer != nil {
		if err := a.tracer.Close(); err != nil {
			a.logger.Error("tracer close", "error", err)
		}
	}

	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Error("redis close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
