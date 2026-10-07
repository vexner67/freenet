package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/reqid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/config"
	postgrpc "github.com/vexner67/freenet/post/internal/grpc"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/notifyclient"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
	"github.com/vexner67/freenet/post/migrations"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// probeTimeout bounds the healthcheck.
	probeTimeout   = 3 * time.Second
	connectTimeout = 5 * time.Second
	migrateTimeout = time.Minute
)

type App struct {
	logger           *slog.Logger
	pool             *pgxpool.Pool
	limiter          ratelimit.Store
	notificationConn *grpc.ClientConn
	outbox           *outbox.Postgres
	relay            *outbox.Relay
	server           *grpcserver.Server
	metrics          *metrics.Server
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
	reg := metrics.NewRegistry()

	repos, err := a.newRepositories(cfg)
	if err != nil {
		return errs.Wrap("a.newRepositories", err)
	}

	notifier, err := a.newNotifier(cfg, repos.outbox, reg)
	if err != nil {
		return errs.Wrap("a.newNotifier", err)
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
		a.logger.Warn("mTLS is off: the service accepts plaintext connections, do not use in production")
	}

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(
			postgrpc.ErrorInterceptor(a.logger),
			jwtauth.UnaryServerInterceptor(
				jwtauth.NewVerifier(publicKey, cfg.JWTIssuer),
				postgrpc.PublicMethods()...,
			),
			ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger),
		),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}
	postv1.RegisterPostServiceServer(
		a.server.Registrar(),
		postgrpc.NewPostHandler(
			service.NewPostService(repos.posts, repos.likes, repos.tx, notifier),
			service.NewCommentService(repos.posts, repos.comments, repos.tx, notifier),
			a.logger,
		),
	)

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("post configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(),
		"notifications", cfg.NotificationServiceAddr != "")

	return nil
}

// rateLimits are what each user may write how often.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		postv1.PostService_CreatePost_FullMethodName:    cfg.PostLimits,
		postv1.PostService_RepostPost_FullMethodName:    cfg.PostLimits,
		postv1.PostService_CreateComment_FullMethodName: cfg.CommentLimits,
		postv1.PostService_LikePost_FullMethodName:      cfg.LikeLimits,
		postv1.PostService_UnlikePost_FullMethodName:    cfg.LikeLimits,
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

// newNotifier records notifications in the outbox and sets up the relay
// that delivers them to the notification service. The connection is lazy,
// so the services may start in any order; until notification is up, the
// relay retries.
func (a *App) newNotifier(cfg config.Config, queue outboxQueue, reg prometheus.Registerer) (service.Notifier, error) {
	if cfg.NotificationServiceAddr == "" {
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: likes and comments notify nobody")
		return notifyclient.Nop{}, nil
	}

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	a.notificationConn, err = grpc.NewClient(cfg.NotificationServiceAddr, append(reqid.DialOptions(), grpc.WithTransportCredentials(creds))...)
	if err != nil {
		return nil, errs.Wrap("grpc.NewClient", err)
	}

	deliverer := notifyclient.NewDeliverer(notificationv1.NewNotificationServiceClient(a.notificationConn), cfg.NotifyTimeout)

	a.relay, err = outbox.NewRelay(queue, deliverer.Deliver, reg, a.logger)
	if err != nil {
		return nil, errs.Wrap("outbox.NewRelay", err)
	}

	return notifyclient.NewOutbox(queue), nil
}

// outboxQueue is the outbox of the storage in use.
type outboxQueue interface {
	outbox.Queue
	notifyclient.Recorder
}

type repositories struct {
	posts    service.PostRepository
	likes    service.LikeRepository
	comments service.CommentRepository
	tx       service.TxManager
	outbox   outboxQueue
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		return repositories{
			posts:    store.Posts(),
			likes:    store.Likes(),
			comments: store.Comments(),
			tx:       inmem.TxManager{},
			outbox:   outbox.NewMemory(),
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

	a.outbox = outbox.NewPostgres(a.pool)

	return repositories{
		posts:    postgres.NewPostRepository(a.pool),
		likes:    postgres.NewLikeRepository(a.pool),
		comments: postgres.NewCommentRepository(a.pool),
		tx:       pgpool.NewTxManager(a.pool),
		outbox:   a.outbox,
	}, nil
}

// Run serves and delivers notifications until ctx is cancelled or the gRPC
// or metrics server fails, which stops the rest too.
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

	if a.relay != nil {
		g.Go(func() error {
			return errs.Wrap("a.relay.Run", a.relay.Run(ctx))
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
	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
	}

	// After Run: the relay owns the outbox connection while it runs.
	if a.outbox != nil {
		a.outbox.Close()
	}

	if a.notificationConn != nil {
		if err := a.notificationConn.Close(); err != nil {
			a.logger.Error("notification connection close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
