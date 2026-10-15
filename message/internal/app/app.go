package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	goredis "github.com/redis/go-redis/v9"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"github.com/vexner67/freenet/message/internal/config"
	messagegrpc "github.com/vexner67/freenet/message/internal/grpc"
	"github.com/vexner67/freenet/message/internal/inmem"
	"github.com/vexner67/freenet/message/internal/mediaclient"
	"github.com/vexner67/freenet/message/internal/postgres"
	messageredis "github.com/vexner67/freenet/message/internal/redis"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/message/migrations"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcclient"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/pubsub"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/tracing"
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
	tracer  *tracing.Tracer
	pool    *pgxpool.Pool
	limiter ratelimit.Store
	// mediaConn, queue and relay tell media about the photos of messages.
	mediaConn *grpc.ClientConn
	queue     *outbox.Postgres
	relay     *outbox.Relay
	redis     *goredis.Client
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
	tracer, err := tracing.New(context.Background(), "message", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer

	reg := metrics.NewRegistry()

	repos, err := a.newRepositories(cfg)
	if err != nil {
		return errs.Wrap("a.newRepositories", err)
	}

	photos, err := a.newPhotoKeeper(cfg, repos.outbox, reg)
	if err != nil {
		return errs.Wrap("a.newPhotoKeeper", err)
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

	events := a.newEvents(cfg)

	messagev1.RegisterMessageServiceServer(
		a.server.Registrar(),
		messagegrpc.NewMessageHandler(
			service.NewMessageService(service.Deps{
				Messages: repos.messages, Dialogs: repos.dialogs, Keys: repos.keys, ChatMessages: repos.chatMessages, ChatMembers: repos.chatMembers,
				Events: events, Tx: repos.tx, Photos: photos,
			}),
			service.NewChatService(service.ChatDeps{
				Chats: repos.chats, Members: repos.chatMembers, Messages: repos.chatMessages, DialogMessages: repos.messages,
				Events: events, Tx: repos.tx, Photos: photos,
			}),
			a.logger,
		),
	)

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("message configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(), "media", cfg.MediaServiceAddr != "")

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

// newPhotoKeeper records in the outbox what media has to do with the
// photos of messages and sets up the relay that tells it. The connection is
// lazy, so the services may start in any order; until media is up, the
// relay retries.
func (a *App) newPhotoKeeper(cfg config.Config, queue outboxQueue, reg prometheus.Registerer) (service.PhotoKeeper, error) {
	if cfg.MediaServiceAddr == "" {
		a.logger.Warn("MEDIA_SERVICE_ADDR is not set: media deletes the photos of messages as never sent")
		return mediaclient.Nop{}, nil
	}

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	a.mediaConn, err = grpcclient.New(cfg.MediaServiceAddr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, errs.Wrap("grpcclient.New", err)
	}

	deliverer := mediaclient.NewDeliverer(mediav1.NewMediaServiceClient(a.mediaConn), cfg.MediaTimeout)

	// As in post and user, whose streams share the metrics, told apart by
	// a label.
	a.relay, err = outbox.NewRelay(queue, deliverer.Deliver, prometheus.WrapRegistererWith(prometheus.Labels{"stream": mediaclient.Stream}, reg), a.logger)
	if err != nil {
		return nil, errs.Wrap("outbox.NewRelay", err)
	}

	return mediaclient.NewOutbox(queue), nil
}

// outboxQueue is the outbox of the storage in use.
type outboxQueue interface {
	outbox.Queue
	mediaclient.Recorder
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
		// An edit is a message sent again; it has its own counter.
		messagev1.MessageService_EditMessage_FullMethodName: cfg.MessageLimits,
		// A message of a chat is a message; each has its own counter.
		messagev1.MessageService_SendChatMessage_FullMethodName: cfg.MessageLimits,
		messagev1.MessageService_EditChatMessage_FullMethodName: cfg.MessageLimits,
		messagev1.MessageService_CreateChat_FullMethodName:      cfg.ChatLimits,
		messagev1.MessageService_AddChatMembers_FullMethodName:  cfg.ChatLimits,
		messagev1.MessageService_RenameChat_FullMethodName:      cfg.ChatLimits,
		messagev1.MessageService_PinChatMessage_FullMethodName:  cfg.ChatLimits,
		messagev1.MessageService_SetChatPhoto_FullMethodName:    cfg.ChatLimits,
		// Typing tells others nothing to keep, but it is told to them.
		messagev1.MessageService_SetTyping_FullMethodName:     cfg.TypingLimits,
		messagev1.MessageService_SetChatTyping_FullMethodName: cfg.TypingLimits,
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
	keys     service.KeyRepository
	tx       service.TxManager
	outbox   outboxQueue

	chats        service.ChatRepository
	chatMembers  service.ChatMemberRepository
	chatMessages service.ChatMessageRepository
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		return repositories{
			messages: store.Messages(),
			dialogs:  store.Dialogs(),
			keys:     store.Keys(),
			tx:       inmem.TxManager{},
			outbox:   outbox.NewMemory(),

			chats:        store.Chats(),
			chatMembers:  store.ChatMembers(),
			chatMessages: store.ChatMessages(),
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

	a.queue = outbox.NewPostgres(a.pool, mediaclient.Stream)

	return repositories{
		messages: postgres.NewMessageRepository(a.pool),
		dialogs:  postgres.NewDialogRepository(a.pool),
		keys:     postgres.NewKeyRepository(a.pool),
		tx:       pgpool.NewTxManager(a.pool),
		outbox:   a.queue,

		chats:        postgres.NewChatRepository(a.pool),
		chatMembers:  postgres.NewChatMemberRepository(a.pool),
		chatMessages: postgres.NewChatMessageRepository(a.pool),
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

	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
	}
	// After Run: the relay owns the outbox connection while it runs.
	if a.queue != nil {
		a.queue.Close()
	}

	if a.mediaConn != nil {
		if err := a.mediaConn.Close(); err != nil {
			a.logger.Error("media connection close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
