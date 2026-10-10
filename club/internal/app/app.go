// Package app is the composition root of the club service.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/club/internal/config"
	clubgrpc "github.com/vexner67/freenet/club/internal/grpc"
	"github.com/vexner67/freenet/club/internal/inmem"
	"github.com/vexner67/freenet/club/internal/notifyclient"
	"github.com/vexner67/freenet/club/internal/postgres"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/club/migrations"
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
	logger  *slog.Logger
	pool    *pgxpool.Pool
	limiter ratelimit.Store
	// notifyConn, queue and relay tell notification about appointments.
	notifyConn *grpc.ClientConn
	queue      *outbox.Postgres
	relay      *outbox.Relay
	server     *grpcserver.Server
	metrics    *metrics.Server
	addr       string
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

	store, err := a.newStorage(cfg)
	if err != nil {
		return errs.Wrap("a.newStorage", err)
	}

	notifier, err := a.newNotifier(cfg, store.outbox, reg)
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
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	verifier := jwtauth.NewVerifier(publicKey, cfg.JWTIssuer)

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(
			clubgrpc.ErrorInterceptor(a.logger),
			restrictInternal(cfg),
			jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: clubgrpc.PublicMethods()}),
			ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger),
		),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}

	clubv1.RegisterClubServiceServer(a.server.Registrar(), clubgrpc.NewClubHandler(service.NewClubService(service.ClubDeps{
		Clubs: store.clubs, Members: store.members, Tx: store.tx, Notifier: notifier,
	})))

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("club configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(), "notifications", cfg.NotificationServiceAddr != "")

	return nil
}

// restrictInternal lets only their services call the internal methods; with
// mTLS off, which is for development, nobody can tell who calls.
func restrictInternal(cfg config.Config) grpc.UnaryServerInterceptor {
	if !cfg.TLS.Enabled() {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(ctx, req)
		}
	}

	return mtls.RestrictMethods(clubgrpc.InternalMethods())
}

// rateLimits are how often each user may make and change clubs and join
// them.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		clubv1.ClubService_CreateClub_FullMethodName: cfg.ClubLimits,
		// An event is a club, under the same limit with a counter of its own.
		clubv1.ClubService_CreateEvent_FullMethodName: cfg.ClubLimits,
		clubv1.ClubService_UpdateClub_FullMethodName:  cfg.EditLimits,
		// Handing a club over is rare; it counts as a change of settings.
		clubv1.ClubService_TransferClub_FullMethodName:   cfg.EditLimits,
		clubv1.ClubService_DescribeMember_FullMethodName: cfg.EditLimits,
		// A photo is uploaded to media first, under its own limit; this
		// counts as one more change of the settings.
		clubv1.ClubService_SetClubAvatar_FullMethodName:   cfg.EditLimits,
		clubv1.ClubService_SetClubBackdrop_FullMethodName: cfg.EditLimits,
		clubv1.ClubService_JoinClub_FullMethodName:        cfg.JoinLimits,
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

// newNotifier records appointments in the outbox and sets up the relay
// that tells the notification service. The connection is lazy, so the
// services may start in any order; until notification is up, the relay
// retries.
func (a *App) newNotifier(cfg config.Config, queue outboxQueue, reg prometheus.Registerer) (service.Notifier, error) {
	if cfg.NotificationServiceAddr == "" {
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: appointments notify nobody")
		return notifyclient.Nop{}, nil
	}

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	a.notifyConn, err = grpc.NewClient(cfg.NotificationServiceAddr, append(reqid.DialOptions(), grpc.WithTransportCredentials(creds))...)
	if err != nil {
		return nil, errs.Wrap("grpc.NewClient", err)
	}

	deliverer := notifyclient.NewDeliverer(notificationv1.NewNotificationServiceClient(a.notifyConn), cfg.NotifyTimeout)

	// As in post and user, whose streams share the metrics, told apart by
	// a label.
	a.relay, err = outbox.NewRelay(queue, deliverer.Deliver, prometheus.WrapRegistererWith(prometheus.Labels{"stream": notifyclient.Stream}, reg), a.logger)
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

// storage is the set of repositories chosen by STORAGE.
type storage struct {
	clubs   service.ClubRepository
	members service.MemberRepository
	tx      service.TxManager
	outbox  outboxQueue
}

func (a *App) newStorage(cfg config.Config) (storage, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		return storage{clubs: store.Clubs(), members: store.Members(), tx: inmem.TxManager{}, outbox: outbox.NewMemory()}, nil
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

	a.queue = outbox.NewPostgres(a.pool, notifyclient.Stream)

	return storage{
		clubs:   postgres.NewClubRepository(a.pool),
		members: postgres.NewMemberRepository(a.pool),
		tx:      pgpool.NewTxManager(a.pool),
		outbox:  a.queue,
	}, nil
}

// Run serves and delivers the outbox until ctx is cancelled or the gRPC
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
	if a.queue != nil {
		a.queue.Close()
	}

	if a.notifyConn != nil {
		if err := a.notifyConn.Close(); err != nil {
			a.logger.Error("notification connection close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
