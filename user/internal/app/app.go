package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/tracing"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/config"
	usergrpc "github.com/vexner67/freenet/user/internal/grpc"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"github.com/vexner67/freenet/user/migrations"
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
	// conns lead to the services told through the outbox, one relay per
	// stream; queues are the PostgreSQL streams, held by the relays.
	conns  []*grpc.ClientConn
	queues []*outbox.Postgres
	relays []*outbox.Relay
	purge  *service.PurgeService
	// sync is nil without a post service to tell.
	sync       *service.SourceSyncService
	syncEvery  time.Duration
	purgeEvery time.Duration
	// presence writes the visits of the people on the site every
	// presenceEvery, all at once.
	presence      *service.PresenceService
	presenceEvery time.Duration
	server        *grpcserver.Server
	metrics       *metrics.Server
	addr          string
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
	tracer, err := tracing.New(context.Background(), "user", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer

	reg := metrics.NewRegistry()

	store, err := a.newStorage(cfg)
	if err != nil {
		return errs.Wrap("a.newStorage", err)
	}

	deliver, err := a.newDelivery(cfg, store.newQueue, store.follows, reg)
	if err != nil {
		return errs.Wrap("a.newDelivery", err)
	}

	notifier := deliver.notifier
	a.purge = service.NewPurgeService(store.users, store.tx, deliver.eraser)
	a.purgeEvery = cfg.AccountPurgeInterval
	if deliver.sources != nil && cfg.FeedSyncInterval > 0 {
		a.sync = service.NewSourceSyncService(store.follows, store.tx, deliver.sources)
		a.syncEvery = cfg.FeedSyncInterval
	}
	a.presence = service.NewPresenceService(store.users)
	a.presenceEvery = cfg.PresenceFlushInterval

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

	interceptors := []grpc.UnaryServerInterceptor{usergrpc.ErrorInterceptor(a.logger)}

	if cfg.TLS.Enabled() {
		interceptors = append(interceptors, mtls.RestrictMethods(usergrpc.InternalMethods()))
	} else {
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	interceptors = append(interceptors, jwtauth.UnaryServerInterceptor(
		jwtauth.NewVerifier(publicKey, cfg.JWTIssuer),
		jwtauth.Access{
			Public:      usergrpc.PublicMethods(),
			Deactivated: usergrpc.DeactivatedMethods(),
			Banned:      usergrpc.BannedMethods(),
		},
	))
	interceptors = append(interceptors, ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger))

	a.server, err = grpcserver.New(a.logger, reg, grpc.Creds(creds), a.tracer.ServerOption(), grpc.ChainUnaryInterceptor(interceptors...))
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}
	permissions := service.NewPermissionService(store.users, store.follows, store.blocks)

	admins, err := user.NewAdmins(cfg.AdminEmails)
	if err != nil {
		return errs.Wrap("user.NewAdmins", err)
	}

	userv1.RegisterUserServiceServer(
		a.server.Registrar(),
		usergrpc.NewUserHandler(usergrpc.Services{
			Users:    service.NewUserService(store.users).TellingWalls(store.tx, deliver.walls),
			Accounts: service.NewAccountService(store.users, store.tx, deliver.reactions, cfg.AccountRetention),
			Follows: service.NewFollowService(service.FollowDeps{
				Users:    store.users,
				Follows:  store.follows,
				Lists:    store.follows,
				Blocks:   store.blocks,
				Invites:  store.users,
				Tx:       store.tx,
				Notifier: notifier,
			}),
			Blocks:  service.NewBlockService(store.users, store.blocks, store.tx, notifier),
			Ignores: service.NewIgnoreService(store.ignores),
			Gifts: service.NewGiftService(service.GiftDeps{
				Users:       store.users,
				Gifts:       store.gifts,
				Prices:      store.gifts,
				Permissions: permissions,
				Tx:          store.tx,
				Wallets:     store.wallets,
				Notifier:    deliver.gifts,
				Admins:      admins,
			}),
			Stickers: service.NewStickerService(store.stickers, store.stickerPacks, store.wallets, store.tx),
			Wallets: service.NewWalletService(service.WalletDeps{
				Users:       store.users,
				Wallets:     store.wallets,
				Vouchers:    store.vouchers,
				Rates:       store.rates,
				Permissions: permissions,
				Tx:          store.tx,
				Notifier:    deliver.votes,
				Admins:      admins,
			}),
			Verification: service.NewVerificationService(service.VerificationDeps{
				Users:         store.users,
				Verifications: store.users,
				Tx:            store.tx,
				Notifier:      deliver.verified,
				Admins:        admins,
			}),
			Bans:          service.NewBanService(store.users, store.users, admins),
			ForeignAgents: service.NewForeignAgentService(store.users, store.users, admins),
			Ads:           service.NewAdService(store.users, store.ads, store.tx, admins),
			Permissions:   permissions,
			Presence:      a.presence,
			Backdrops:     service.NewBackdropService(store.users, store.users),
		}, a.logger),
	)

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("user configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(),
		"notifications", cfg.NotificationServiceAddr != "")

	return nil
}

// rateLimits are how often each user may follow people, send gifts and
// make packs of stickers.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		userv1.UserService_Follow_FullMethodName:   cfg.FollowLimits,
		userv1.UserService_SendGift_FullMethodName: cfg.GiftLimits,
		// An invitation is a follow, counted apart.
		userv1.UserService_AcceptInvite_FullMethodName: cfg.FollowLimits,

		userv1.UserService_TransferVotes_FullMethodName: cfg.TransferLimits,
		userv1.UserService_RedeemVoucher_FullMethodName: cfg.VoucherLimits,

		userv1.UserService_CreateStickerPack_FullMethodName: cfg.StickerPackLimits,
		userv1.UserService_EditStickerPack_FullMethodName:   cfg.StickerPackLimits,
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

// storage is the set of repositories chosen by STORAGE.
type storage struct {
	users interface {
		service.UserRepository
		service.DueUserDeleter
		service.PresenceRepository
		service.BackdropRepository
		service.InviteRepository
		service.VerificationRepository
		service.BanRepository
		service.ForeignAgentRepository
	}
	follows interface {
		service.FollowRepository
		service.FollowListRepository
		service.FollowBook
	}
	blocks  service.BlockRepository
	ignores service.IgnoreRepository
	gifts   interface {
		service.GiftRepository
		service.GiftPriceRepository
	}
	// stickers are the packs of stickers users have taken.
	stickers service.StickerRepository
	// stickerPacks are the packs people make of their photos.
	stickerPacks service.StickerPackRepository
	// wallets are the votes and the rating of users.
	wallets  service.WalletRepository
	vouchers service.VoucherRepository
	// rates is what a vote is worth in rubles.
	rates service.VoteRateRepository
	// ads are the advertising of the global feed.
	ads service.AdRepository
	tx  service.TxManager
	// newQueue opens a stream of the outbox.
	newQueue func(stream string) outboxQueue
}

func (a *App) newStorage(cfg config.Config) (storage, error) {
	if cfg.Storage == config.StorageInmem {
		users := inmem.NewUserRepository()
		follows := inmem.NewFollowRepository(users)
		stickers := inmem.NewStickerRepository(users)
		return storage{
			users:        users,
			follows:      follows,
			blocks:       inmem.NewBlockRepository(follows),
			ignores:      inmem.NewIgnoreRepository(users),
			gifts:        inmem.NewGiftRepository(users),
			stickers:     stickers,
			stickerPacks: inmem.NewStickerPackRepository(users, stickers),
			wallets:      users,
			vouchers:     inmem.NewVoucherRepository(users),
			rates:        inmem.NewVoteRateRepository(),
			ads:          inmem.NewAdRepository(),
			tx:           inmem.TxManager{},
			newQueue: func(string) outboxQueue {
				return outbox.NewMemory()
			},
		}, nil
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
		users:        postgres.NewUserRepository(a.pool),
		follows:      postgres.NewFollowRepository(a.pool),
		blocks:       postgres.NewBlockRepository(a.pool),
		ignores:      postgres.NewIgnoreRepository(a.pool),
		gifts:        postgres.NewGiftRepository(a.pool),
		stickers:     postgres.NewStickerRepository(a.pool),
		stickerPacks: postgres.NewStickerPackRepository(a.pool),
		wallets:      postgres.NewWalletRepository(a.pool),
		vouchers:     postgres.NewVoucherRepository(a.pool),
		rates:        postgres.NewVoteRateRepository(a.pool),
		ads:          postgres.NewAdRepository(a.pool),
		tx:           pgpool.NewTxManager(a.pool),
		newQueue: func(stream string) outboxQueue {
			q := outbox.NewPostgres(a.pool, stream)
			a.queues = append(a.queues, q)
			return q
		},
	}, nil
}

// Run serves, delivers the outbox and erases the pages that are due until
// ctx is cancelled or the gRPC or metrics server fails, which stops the
// rest too.
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

	for _, relay := range a.relays {
		g.Go(func() error {
			return errs.Wrap("relay.Run", relay.Run(ctx))
		})
	}

	g.Go(func() error {
		return errs.Wrap("a.runPurge", a.runPurge(ctx, a.purgeEvery))
	})

	if a.sync != nil {
		g.Go(func() error {
			return errs.Wrap("a.runSync", a.runSync(ctx, a.syncEvery))
		})
	}

	g.Go(func() error {
		a.presence.Run(ctx, a.presenceEvery, func(err error) {
			a.logger.WarnContext(ctx, "visits not recorded", "error", err)
		})

		return nil
	})

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
	// After Run: the relays own the outbox connections while they run.
	for _, q := range a.queues {
		q.Close()
	}

	for _, conn := range a.conns {
		if err := conn.Close(); err != nil {
			a.logger.Error("connection close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
