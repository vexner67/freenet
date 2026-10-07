package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"github.com/vexner67/freenet/message/internal/config"
	messagegrpc "github.com/vexner67/freenet/message/internal/grpc"
	"github.com/vexner67/freenet/message/internal/inmem"
	"github.com/vexner67/freenet/message/internal/postgres"
	messageredis "github.com/vexner67/freenet/message/internal/redis"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/message/migrations"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/pubsub"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// probeTimeout bounds the healthcheck.
	probeTimeout   = 3 * time.Second
	connectTimeout = 5 * time.Second
	// eventBuffer is how far a subscriber may lag before it is dropped
	// and has to catch up from storage.
	eventBuffer    = 64
	migrateTimeout = time.Minute
)

type App struct {
	logger  *slog.Logger
	pool    *pgxpool.Pool
	limiter ratelimit.Store
	redis   *goredis.Client
	// shared carries live events between replicas; nil keeps them here.
	shared  *pubsub.Redis[uuid.UUID, service.Event]
	server  *grpcserver.Server
	metrics *metrics.Server
	addr    string
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

	limiter, err := a.newLimiter(cfg)
	if err != nil {
		return errs.Wrap("a.newLimiter", err)
	}

	publicKey, err := jwtauth.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return errs.Wrap("jwtauth.LoadPublicKey", err)
	}

	creds, err := mtls.ServerCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ServerCredentials", err)
	}

	if !cfg.TLS.Enabled() {
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	verifier := jwtauth.NewVerifier(publicKey, cfg.JWTIssuer)

	reg := metrics.NewRegistry()

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(
			messagegrpc.ErrorInterceptor(a.logger),
			restrictInternal(cfg),
			jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: messagegrpc.PublicMethods()}),
			ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger),
		),
		grpc.ChainStreamInterceptor(jwtauth.StreamServerInterceptor(verifier)),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}

	messagev1.RegisterMessageServiceServer(
		a.server.Registrar(),
		messagegrpc.NewMessageHandler(
			service.NewMessageService(repos.messages, repos.dialogs, a.newEvents(cfg)),
			a.logger,
		),
	)

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("message configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled())

	return nil
}

// newEvents shares live events between the replicas through Redis when it
// is configured; otherwise a stream gets only the events of its replica.
func (a *App) newEvents(cfg config.Config) service.Events {
	if cfg.RedisAddr == "" {
		a.logger.Warn("REDIS_ADDR is not set: live events stay within this replica")
		return pubsub.New[uuid.UUID, service.Event](eventBuffer)
	}

	a.redis = goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	a.shared = pubsub.NewRedis(a.redis, messageredis.Channel, messageredis.EventCodec{}, eventBuffer, a.logger)

	return a.shared
}

// restrictInternal lets only their services call the internal methods; with
// mTLS off, which is for development, nobody can tell who calls.
func restrictInternal(cfg config.Config) grpc.UnaryServerInterceptor {
	if !cfg.TLS.Enabled() {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(ctx, req)
		}
	}

	return mtls.RestrictMethods(messagegrpc.InternalMethods())
}

// rateLimits are how often each user may write.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		messagev1.MessageService_SendMessage_FullMethodName: cfg.MessageLimits,
	}
}

// newLimiter counts in Redis when it is configured, so that all replicas
// share the limits, and in memory otherwise.
func (a *App) newLimiter(cfg config.Config) (ratelimit.Limiter, error) {
	if cfg.RedisAddr == "" {
		a.logger.Warn("REDIS_ADDR is not set: rate limits are counted per replica")
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	var err error

	a.limiter, err = ratelimit.Open(ctx, cfg.RedisAddr)
	if err != nil {
		return nil, errs.Wrap("ratelimit.Open", err)
	}

	return a.limiter, nil
}

type repositories struct {
	messages service.MessageRepository
	dialogs  service.DialogRepository
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		return repositories{messages: store.Messages(), dialogs: store.Dialogs()}, nil
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
		messages: postgres.NewMessageRepository(a.pool),
		dialogs:  postgres.NewDialogRepository(a.pool),
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

	return g.Wait()
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

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Error("redis close", "error", err)
		}
	}

	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
	}
	if a.pool != nil {
		a.pool.Close()
	}
}
