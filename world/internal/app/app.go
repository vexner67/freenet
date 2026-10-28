// Package app is the composition root of the world service.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcclient"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/tracing"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"github.com/vexner67/freenet/world/internal/clubclient"
	"github.com/vexner67/freenet/world/internal/config"
	worldgrpc "github.com/vexner67/freenet/world/internal/grpc"
	"github.com/vexner67/freenet/world/internal/inmem"
	"github.com/vexner67/freenet/world/internal/maker"
	"github.com/vexner67/freenet/world/internal/postgres"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/userclient"
	"github.com/vexner67/freenet/world/migrations"
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
	limiter ratelimit.Store
	conns   []*grpc.ClientConn
	server  *grpcserver.Server
	metrics *metrics.Server
	addr    string

	worlds           *service.WorldService
	games            *service.GameService
	worldInterval    time.Duration
	generateInterval time.Duration
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
		logger:           log,
		addr:             fmt.Sprintf(":%d", cfg.GRPCPort),
		worldInterval:    cfg.WorldInterval,
		generateInterval: cfg.GenerateInterval,
	}

	// On failure Close releases whatever was opened before it.
	if err = a.build(cfg); err != nil {
		a.Close()
		return nil, errs.Wrap("a.build", err)
	}

	return a, nil
}

func (a *App) build(cfg config.Config) error {
	tracer, err := tracing.New(context.Background(), "world", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer

	reg := metrics.NewRegistry()

	store, err := a.newStorage(cfg)
	if err != nil {
		return errs.Wrap("a.newStorage", err)
	}

	wallets, clubs, err := a.newClients(cfg)
	if err != nil {
		return errs.Wrap("a.newClients", err)
	}

	mapMaker, err := maker.New(cfg.TileWidth)
	if err != nil {
		return errs.Wrap("maker.New", err)
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
		a.tracer.ServerOption(),
		grpc.ChainUnaryInterceptor(
			worldgrpc.ErrorInterceptor(a.logger),
			restrictInternal(cfg),
			jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: worldgrpc.PublicMethods()}),
			ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger),
		),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}

	a.worlds = service.NewWorldService(service.WorldDeps{
		Worlds: store.worlds, Maps: store.maps, Games: store.games, Rules: store.rules, Maker: mapMaker, Tx: store.tx,
	})
	a.games = service.NewGameService(service.GameDeps{
		Worlds: a.worlds, Games: store.games, Events: store.events, Wallets: wallets, Clubs: clubs, Tx: store.tx,
	})
	worldv1.RegisterWorldServiceServer(a.server.Registrar(), worldgrpc.NewWorldHandler(a.worlds, a.games))

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("world configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(), "clubs", cfg.ClubServiceAddr != "")

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

	return mtls.RestrictMethods(worldgrpc.InternalMethods())
}

// rateLimits are how often each user may act in the world: all the
// actions count together.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	limits := map[string]ratelimit.Rules{}
	for _, m := range []string{
		worldv1.WorldService_FoundState_FullMethodName,
		worldv1.WorldService_EditState_FullMethodName,
		worldv1.WorldService_ApplyCitizenship_FullMethodName,
		worldv1.WorldService_WithdrawApplication_FullMethodName,
		worldv1.WorldService_AnswerApplication_FullMethodName,
		worldv1.WorldService_LeaveState_FullMethodName,
		worldv1.WorldService_ExpelCitizen_FullMethodName,
		worldv1.WorldService_AppointCitizen_FullMethodName,
		worldv1.WorldService_Abdicate_FullMethodName,
		worldv1.WorldService_Colonize_FullMethodName,
		worldv1.WorldService_Donate_FullMethodName,
	} {
		limits[m] = cfg.ActionLimits
	}

	return limits
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

// newClients connect to the user service, which keeps the votes, and the
// club service, which makes the groups of the states. The connections are
// lazy, so the services may start in any order.
func (a *App) newClients(cfg config.Config) (service.Wallets, service.Clubs, error) {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	userConn, err := grpcclient.New(cfg.UserServiceAddr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, nil, errs.Wrap("grpcclient.New", err)
	}
	a.conns = append(a.conns, userConn)
	wallets := userclient.New(userv1.NewUserServiceClient(userConn), cfg.CallTimeout)

	if cfg.ClubServiceAddr == "" {
		a.logger.Warn("CLUB_SERVICE_ADDR is not set: the states have no groups")
		return wallets, clubclient.Nop{}, nil
	}

	clubConn, err := grpcclient.New(cfg.ClubServiceAddr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, nil, errs.Wrap("grpcclient.New", err)
	}
	a.conns = append(a.conns, clubConn)

	return wallets, clubclient.New(clubv1.NewClubServiceClient(clubConn), cfg.CallTimeout), nil
}

// storage is the set of repositories chosen by STORAGE.
type storage struct {
	worlds service.WorldRepository
	maps   service.MapRepository
	games  service.GameRepository
	events service.EventRepository
	rules  service.RulesRepository
	tx     service.TxManager
}

func (a *App) newStorage(cfg config.Config) (storage, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		return storage{worlds: store, maps: store, games: store, events: store, rules: store, tx: inmem.TxManager{}}, nil
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
		worlds: postgres.NewWorldRepository(a.pool),
		maps:   postgres.NewMapRepository(a.pool),
		games:  postgres.NewGameRepository(a.pool),
		events: postgres.NewEventRepository(a.pool),
		rules:  postgres.NewRulesRepository(a.pool),
		tx:     pgpool.NewTxManager(a.pool),
	}, nil
}

// Run serves, counts the hours of the world and makes the maps until ctx
// is cancelled or the gRPC or metrics server fails, which stops the rest
// too.
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
		a.runWorld(ctx)
		return nil
	})

	g.Go(func() error {
		a.runGeneration(ctx)
		return nil
	})

	return g.Wait()
}

// runWorld counts the hours of the open world and makes the groups of new
// states every interval until ctx is cancelled; a failure is logged and
// the next round tries again.
func (a *App) runWorld(ctx context.Context) {
	ticker := time.NewTicker(a.worldInterval)
	defer ticker.Stop()

	for {
		if hours, err := a.games.Tick(ctx); err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "hour of the world", "error", err)
		} else if hours > 0 {
			a.logger.InfoContext(ctx, "hours of the world counted", "hours", hours)
		}

		if _, err := a.games.MakeClubs(ctx); err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "groups of the states", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runGeneration makes the maps of new worlds, when told of one and every
// interval, until ctx is cancelled.
func (a *App) runGeneration(ctx context.Context) {
	ticker := time.NewTicker(a.generateInterval)
	defer ticker.Stop()

	for {
		for {
			start := time.Now()
			made, err := a.worlds.Generate(ctx)
			if err != nil && ctx.Err() == nil {
				a.logger.ErrorContext(ctx, "map of a world", "error", err)
			}
			if !made {
				break
			}
			a.logger.InfoContext(ctx, "map of a world made", "took", time.Since(start).Round(time.Millisecond), "ok", err == nil)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-a.worlds.Wake():
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
	if a.tracer != nil {
		if err := a.tracer.Close(); err != nil {
			a.logger.Error("tracer close", "error", err)
		}
	}

	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
	}

	for _, conn := range a.conns {
		if err := conn.Close(); err != nil {
			a.logger.Error("service connection close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
