package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/federation/internal/config"
	"github.com/vexner67/freenet/federation/internal/delivery"
	"github.com/vexner67/freenet/federation/internal/fedi"
	federationgrpc "github.com/vexner67/freenet/federation/internal/grpc"
	"github.com/vexner67/freenet/federation/internal/httpsig"
	"github.com/vexner67/freenet/federation/internal/inmem"
	"github.com/vexner67/freenet/federation/internal/postclient"
	"github.com/vexner67/freenet/federation/internal/postgres"
	"github.com/vexner67/freenet/federation/internal/remote"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/federation/internal/userclient"
	"github.com/vexner67/freenet/federation/migrations"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcclient"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/tracing"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
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
	tracer  *tracing.Tracer
	pool    *pgxpool.Pool
	server  *grpcserver.Server
	metrics *metrics.Server
	addr    string
	// relay sends the activities of the pages to other servers.
	relay   *outbox.Relay
	queue   *outbox.Postgres
	limiter *ratelimit.Memory
	conns   []*grpc.ClientConn
	// news forgets old remote posts every cleanup.
	news    *service.Service
	cleanup time.Duration
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
	tracer, err := tracing.New(context.Background(), "federation", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer

	site, err := fedi.NewSite(cfg.PublicURL)
	if err != nil {
		return errs.Wrap("fedi.NewSite", err)
	}

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

	interceptors := []grpc.UnaryServerInterceptor{federationgrpc.ErrorInterceptor(a.logger)}

	if cfg.TLS.Enabled() {
		interceptors = append(interceptors, mtls.RestrictMethods(federationgrpc.InternalMethods()))
	} else {
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	verifier := jwtauth.NewVerifier(publicKey, cfg.JWTIssuer)
	interceptors = append(interceptors, jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: federationgrpc.PublicMethods()}))

	reg := metrics.NewRegistry()

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		a.tracer.ServerOption(),
		grpc.ChainUnaryInterceptor(interceptors...),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}

	users, posts, err := a.newClients(cfg)
	if err != nil {
		return errs.Wrap("a.newClients", err)
	}

	client := remote.New(site, cfg.RemoteTimeout)

	deliverer := delivery.NewDeliverer(repos.accounts, client, cfg.DeliveryMaxAge)

	a.relay, err = outbox.NewRelay(repos.queue, deliverer.Deliver, prometheus.WrapRegistererWith(prometheus.Labels{"stream": delivery.Stream}, reg),
		a.logger, outbox.Keyed(delivery.Key))
	if err != nil {
		return errs.Wrap("outbox.NewRelay", err)
	}

	a.limiter = ratelimit.NewMemory()

	federation := service.New(service.Deps{
		Site:       site,
		Accounts:   repos.accounts,
		Followers:  repos.followers,
		Remotes:    repos.remotes,
		People:     users,
		Posts:      posts,
		Resolver:   client,
		Signatures: httpsig.Signatures{},
		Sender:     delivery.NewOutbox(repos.queue),
		Keys:       httpsig.Keys{},
		Limiter:    a.limiter,
		Tx:         repos.tx,

		Following:     repos.following,
		Subscriptions: repos.subscriptions,
		News:          repos.news,
		Finder:        client,
		Media:         client,
		Languages:     cfg.FeedLanguages,
		Retention:     cfg.NewsRetention,
	})
	a.news, a.cleanup = federation, cfg.CleanupInterval

	federationv1.RegisterFederationServiceServer(a.server.Registrar(), federationgrpc.NewFederationHandler(federation, federation, site))

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("federation configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(), "domain", site.Domain())

	return nil
}

// newClients connects to the user and post services; the connections
// are lazy, so the services may start in any order.
func (a *App) newClients(cfg config.Config) (*userclient.Client, *postclient.Client, error) {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	users, err := grpcclient.New(cfg.UserServiceAddr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, nil, errs.Wrap("grpcclient.New", err)
	}
	a.conns = append(a.conns, users)

	posts, err := grpcclient.New(cfg.PostServiceAddr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, nil, errs.Wrap("grpcclient.New", err)
	}
	a.conns = append(a.conns, posts)

	return userclient.New(userv1.NewUserServiceClient(users), cfg.ServicesTimeout),
		postclient.New(postv1.NewPostServiceClient(posts), cfg.ServicesTimeout), nil
}

// outboxQueue is the outbox of the storage in use.
type outboxQueue interface {
	outbox.Queue
	delivery.Recorder
}

type repositories struct {
	accounts  service.AccountRepository
	followers service.FollowerRepository
	remotes   service.RemoteRepository
	tx        service.TxManager
	queue     outboxQueue

	following     service.FollowingRepository
	subscriptions service.SubscriptionRepository
	news          service.RemotePostRepository
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		following, subscriptions := inmem.NewFollowingRepository(), inmem.NewSubscriptionRepository()

		return repositories{
			accounts:  inmem.NewAccountRepository(),
			followers: inmem.NewFollowerRepository(),
			remotes:   inmem.NewRemoteRepository(),
			tx:        inmem.TxManager{},
			queue:     outbox.NewMemory(),

			following:     following,
			subscriptions: subscriptions,
			news:          inmem.NewRemotePostRepository(following, subscriptions),
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

	a.queue = outbox.NewPostgres(a.pool, delivery.Stream)

	return repositories{
		accounts:  postgres.NewAccountRepository(a.pool),
		followers: postgres.NewFollowerRepository(a.pool),
		remotes:   postgres.NewRemoteRepository(a.pool),
		tx:        pgpool.NewTxManager(a.pool),
		queue:     a.queue,

		following:     postgres.NewFollowingRepository(a.pool),
		subscriptions: postgres.NewSubscriptionRepository(a.pool),
		news:          postgres.NewRemotePostRepository(a.pool),
	}, nil
}

// Run serves and sends activities until ctx is cancelled or the gRPC or
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

	g.Go(func() error {
		return errs.Wrap("a.relay.Run", a.relay.Run(ctx))
	})

	g.Go(func() error {
		a.forgetOldNews(ctx)
		return nil
	})

	return g.Wait()
}

// forgetOldNews forgets the remote posts older than the news keep, every
// cleanup interval until ctx is cancelled; a failure waits for the next.
func (a *App) forgetOldNews(ctx context.Context) {
	ticker := time.NewTicker(a.cleanup)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		n, err := a.news.Cleanup(ctx)
		if err != nil {
			a.logger.ErrorContext(ctx, "forget old news", "error", err)
			continue
		}

		if n > 0 {
			a.logger.InfoContext(ctx, "old news forgotten", "count", n)
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

	for _, conn := range a.conns {
		if err := conn.Close(); err != nil {
			a.logger.Error("connection close", "error", err, "target", conn.Target())
		}
	}

	if a.tracer != nil {
		if err := a.tracer.Close(); err != nil {
			a.logger.Error("tracer close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
