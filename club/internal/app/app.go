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
	"github.com/vexner67/freenet/club/internal/postclient"
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
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
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
	// conns lead to the services told through the outbox — notification
	// about appointments, post about memberships — one stream and one
	// relay each.
	conns   []*grpc.ClientConn
	queues  []*outbox.Postgres
	relays  []*outbox.Relay
	server  *grpcserver.Server
	metrics *metrics.Server
	addr    string
	// reminder tells the members of events that they start soon.
	reminder *service.ReminderService
	// sync is nil without a post service to tell.
	sync        *service.ReaderSyncService
	syncEvery   time.Duration
	remindEvery time.Duration
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

	notifier, err := a.newNotifier(cfg, store.newQueue, reg)
	if err != nil {
		return errs.Wrap("a.newNotifier", err)
	}

	readers, rosters, err := a.newReaders(cfg, store, reg)
	if err != nil {
		return errs.Wrap("a.newReaders", err)
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
		Clubs: store.clubs, Members: store.members, Requests: store.requests, Tx: store.tx, Notifier: notifier, Readers: readers,
	})))

	a.reminder = service.NewReminderService(store.reminders, store.tx, notifier, cfg.EventReminderAhead, time.Now)
	a.remindEvery = cfg.EventReminderInterval
	if rosters != nil && cfg.FeedSyncInterval > 0 {
		a.sync = service.NewReaderSyncService(store.book, store.tx, rosters)
		a.syncEvery = cfg.FeedSyncInterval
	}

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("club configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(), "notifications", cfg.NotificationServiceAddr != "", "news", cfg.PostServiceAddr != "")

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
func (a *App) newNotifier(cfg config.Config, newQueue func(stream string) outboxQueue, reg prometheus.Registerer) (notifier, error) {
	if cfg.NotificationServiceAddr == "" {
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: appointments and events that start soon notify nobody")
		return notifyclient.Nop{}, nil
	}

	queue, err := a.newStream(cfg, notifyclient.Stream, cfg.NotificationServiceAddr, newQueue, reg, func(conn *grpc.ClientConn) outbox.Deliver {
		return notifyclient.NewDeliverer(notificationv1.NewNotificationServiceClient(conn), cfg.NotifyTimeout).Deliver
	})
	if err != nil {
		return nil, errs.Wrap("a.newStream", err)
	}

	return notifyclient.NewOutbox(queue), nil
}

// newReaders records memberships in the outbox and sets up the relay that
// tells the post service, whose news show a member the wall of the club.
// The rosters, nil without a post service, tell it who is in every club
// now and then.
func (a *App) newReaders(cfg config.Config, store storage, reg prometheus.Registerer) (service.Readers, service.Rosters, error) {
	if cfg.PostServiceAddr == "" {
		a.logger.Warn("POST_SERVICE_ADDR is not set: the news do not show the walls of clubs")
		return postclient.Nop{}, nil, nil
	}

	queue, err := a.newStream(cfg, postclient.Stream, cfg.PostServiceAddr, store.newQueue, reg, func(conn *grpc.ClientConn) outbox.Deliver {
		return postclient.Deliver(postv1.NewPostServiceClient(conn), cfg.NotifyTimeout)
	})
	if err != nil {
		return nil, nil, errs.Wrap("a.newStream", err)
	}

	toPost := postclient.NewOutbox(queue, store.book)

	return toPost, toPost, nil
}

// newStream opens the outbox stream of the service at addr and the relay
// that delivers it.
func (a *App) newStream(
	cfg config.Config, name, addr string, newQueue func(stream string) outboxQueue, reg prometheus.Registerer,
	deliver func(*grpc.ClientConn) outbox.Deliver,
) (outboxQueue, error) {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	conn, err := grpc.NewClient(addr, append(reqid.DialOptions(), grpc.WithTransportCredentials(creds))...)
	if err != nil {
		return nil, errs.Wrap("grpc.NewClient", err)
	}
	a.conns = append(a.conns, conn)

	queue := newQueue(name)

	// As in post and user, the streams share the metrics, told apart by a
	// label.
	relay, err := outbox.NewRelay(queue, deliver(conn), prometheus.WrapRegistererWith(prometheus.Labels{"stream": name}, reg), a.logger)
	if err != nil {
		return nil, errs.Wrap("outbox.NewRelay", err)
	}
	a.relays = append(a.relays, relay)

	return queue, nil
}

// notifier is what the notification service hears from club.
type notifier interface {
	service.Notifier
	service.Reminder
}

// outboxQueue is a stream of the outbox of the storage in use.
type outboxQueue interface {
	outbox.Queue
	notifyclient.Recorder
}

// storage is the set of repositories chosen by STORAGE.
type storage struct {
	clubs     service.ClubRepository
	members   service.MemberRepository
	requests  service.RequestRepository
	reminders service.DueEvents
	// book reads the memberships for post to match its copy of them.
	book service.MemberBook
	tx   service.TxManager
	// newQueue opens a stream of the outbox.
	newQueue func(stream string) outboxQueue
}

func (a *App) newStorage(cfg config.Config) (storage, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		return storage{
			clubs: store.Clubs(), members: store.Members(), requests: store.Requests(), reminders: store.Reminders(), book: store.Book(), tx: inmem.TxManager{},
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
		clubs:     postgres.NewClubRepository(a.pool),
		members:   postgres.NewMemberRepository(a.pool),
		requests:  postgres.NewRequestRepository(a.pool),
		reminders: postgres.NewReminderRepository(a.pool),
		book:      postgres.NewBookRepository(a.pool),
		tx:        pgpool.NewTxManager(a.pool),
		newQueue: func(stream string) outboxQueue {
			q := outbox.NewPostgres(a.pool, stream)
			a.queues = append(a.queues, q)

			return q
		},
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

	for _, relay := range a.relays {
		g.Go(func() error {
			return errs.Wrap("relay.Run", relay.Run(ctx))
		})
	}

	g.Go(func() error {
		return errs.Wrap("a.runReminder", a.runReminder(ctx))
	})

	if a.sync != nil {
		g.Go(func() error {
			return errs.Wrap("a.runSync", a.runSync(ctx))
		})
	}

	return g.Wait()
}

// runSync tells post who is in every club every interval until ctx is
// cancelled; a failure is logged and the next round starts over.
func (a *App) runSync(ctx context.Context) error {
	ticker := time.NewTicker(a.syncEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		told, err := a.sync.SyncAll(ctx)
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "sync of feed sources", "error", err)
		}

		a.logger.InfoContext(ctx, "members told to post anew", "clubs", told)
	}
}

// runReminder reminds of the events that start soon until ctx is
// cancelled. A failed round is logged and tried again next time: the
// events stay due.
func (a *App) runReminder(ctx context.Context) error {
	ticker := time.NewTicker(a.remindEvery)
	defer ticker.Stop()

	for {
		reminded, err := a.reminder.RemindDue(ctx)

		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			a.logger.ErrorContext(ctx, "remind of events", "error", err)
		case reminded > 0:
			a.logger.InfoContext(ctx, "events reminded of", "count", reminded)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
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

	// After Run: the relays own the outbox connections while they run.
	for _, q := range a.queues {
		q.Close()
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
