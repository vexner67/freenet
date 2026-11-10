package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	redis "github.com/redis/go-redis/v9"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
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
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
	"github.com/vexner67/freenet/room/internal/clubclient"
	"github.com/vexner67/freenet/room/internal/config"
	roomgrpc "github.com/vexner67/freenet/room/internal/grpc"
	"github.com/vexner67/freenet/room/internal/inmem"
	"github.com/vexner67/freenet/room/internal/livekit"
	roommetrics "github.com/vexner67/freenet/room/internal/metrics"
	"github.com/vexner67/freenet/room/internal/notifyclient"
	"github.com/vexner67/freenet/room/internal/postgres"
	roomredis "github.com/vexner67/freenet/room/internal/redis"
	"github.com/vexner67/freenet/room/internal/service"
	"github.com/vexner67/freenet/room/internal/userclient"
	"github.com/vexner67/freenet/room/migrations"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

type queue interface {
	outbox.Queue
	notifyclient.Recorder
}
type App struct {
	mediaQueue *outbox.Postgres
	mediaRelay *outbox.Relay
	logger     *slog.Logger
	tracer     *tracing.Tracer
	pool       *pgxpool.Pool
	redis      *redis.Client
	server     *grpcserver.Server
	metrics    *metrics.Server
	shared     *pubsub.Redis[uuid.UUID, service.Event]
	hub        *pubsub.Hub[uuid.UUID, service.Event]
	limiter    ratelimit.Store
	queue      *outbox.Postgres
	relay      *outbox.Relay
	rooms      *service.Service
	conns      []*grpc.ClientConn
	addr       string
	sweepEvery time.Duration
}

func New(cfg config.Config) (*App, error) {
	log, err := logger.New(logger.Config{LogLevel: cfg.LogLevel, LogFormat: cfg.LogFormat})
	if err != nil {
		return nil, errs.Wrap("logger.New", err)
	}
	a := &App{logger: log, addr: fmt.Sprintf(":%d", cfg.GRPCPort), sweepEvery: cfg.SweepInterval}
	if err = a.build(cfg); err != nil {
		a.Close()
		return nil, errs.Wrap("a.build", err)
	}
	return a, nil
}
func (a *App) build(cfg config.Config) error {
	tracer, err := tracing.New(context.Background(), "room", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer
	reg := metrics.NewRegistry()
	deps, q, err := a.storage(cfg)
	if err != nil {
		return errs.Wrap("a.storage", err)
	}
	deps.Clock = clock{}
	deps.Observer = roommetrics.New(reg)
	deps.MaxMembers, deps.MaxLive, deps.EmptyTTL = cfg.MaxMembers, cfg.MaxLive, cfg.EmptyTTL
	mediaClient := livekit.New(livekit.Config{URL: cfg.LiveKitURL, PublicURL: cfg.LiveKitPublicURL, APIKey: cfg.LiveKitKey, APISecret: cfg.LiveKitSecret, Timeout: cfg.LiveKitTimeout, TokenTTL: cfg.LiveKitTokenTTL, EmptyTTL: cfg.EmptyTTL, MaxMembers: cfg.MaxMembers}, clock{})
	deps.Media, deps.MediaTokens = mediaClient, mediaClient
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ClientCredentials", err)
	}
	addresses := []string{cfg.UserAddr, cfg.ClubAddr, cfg.NotificationAddr}
	for _, addr := range addresses {
		conn, err := grpcclient.New(addr, creds, a.tracer.DialOption())
		if err != nil {
			return errs.Wrap("grpcclient.New", err)
		}
		a.conns = append(a.conns, conn)
	}
	deps.Users = userclient.New(userv1.NewUserServiceClient(a.conns[0]), cfg.ServicesTimeout)
	deps.Clubs = clubclient.New(clubv1.NewClubServiceClient(a.conns[1]), cfg.ServicesTimeout)
	deps.Notify = notifyclient.NewOutbox(q)
	deliverer := notifyclient.NewDeliverer(notificationv1.NewNotificationServiceClient(a.conns[2]), cfg.ServicesTimeout)
	a.relay, err = outbox.NewRelay(q, deliverer.Deliver, prometheus.WrapRegistererWith(prometheus.Labels{"stream": notifyclient.Stream}, reg), a.logger)
	if err != nil {
		return errs.Wrap("outbox.NewRelay", err)
	}
	var mediaQueue queue
	if a.pool != nil {
		a.mediaQueue = outbox.NewPostgres(a.pool, livekit.Stream)
		mediaQueue = a.mediaQueue
	} else {
		mediaQueue = outbox.NewMemory()
	}
	reliable := livekit.NewReliable(deps.Media, mediaQueue, deps.Index, deps.Bans, clock{}, a.logger)
	deps.Media = reliable
	a.mediaRelay, err = outbox.NewRelay(mediaQueue, reliable.Deliver, prometheus.WrapRegistererWith(prometheus.Labels{"stream": livekit.Stream}, reg), a.logger)
	if err != nil {
		return errs.Wrap("outbox.NewRelay", err)
	}

	a.rooms = service.New(deps)
	publicKey, err := jwtauth.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return errs.Wrap("jwtauth.LoadPublicKey", err)
	}
	verifier := jwtauth.NewVerifier(publicKey, cfg.JWTIssuer)
	serverCreds, err := mtls.ServerCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ServerCredentials", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()
	a.limiter, err = ratelimit.Open(ctx, cfg.RedisAddr)
	if err != nil {
		return errs.Wrap("ratelimit.Open", err)
	}
	a.server, err = grpcserver.New(a.logger, reg, grpc.Creds(serverCreds), a.tracer.ServerOption(), grpc.ChainUnaryInterceptor(roomgrpc.ErrorInterceptor(a.logger), roomgrpc.AuthInterceptor(verifier), ratelimit.UnaryServerInterceptor(a.limiter, rateLimits(cfg), a.logger)), grpc.ChainStreamInterceptor(jwtauth.StreamServerInterceptor(verifier)))
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}
	h := roomgrpc.NewHandler(roomgrpc.Deps{Reader: a.rooms, Editor: a.rooms, Members: a.rooms, Content: a.rooms, Management: a.rooms, Cleaner: a.rooms, MediaAccess: a.rooms, Events: deps.Events, MaxMembers: cfg.MaxMembers})
	roomv1.RegisterRoomServiceServer(a.server.Registrar(), h)
	if cfg.MetricsPort != 0 {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}
	if !cfg.TLS.Enabled() {
		a.logger.Warn("rooms require mTLS to query user and club relationships; use the rooms development stack")
	}
	return nil
}
func (a *App) storage(cfg config.Config) (service.Deps, queue, error) {
	d := service.Deps{}
	var q queue
	if cfg.Storage == config.StoragePersistent {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.MigrationTimeout)
		defer cancel()
		if cfg.MigrateOnStart {
			if _, err := pgpool.Migrate(ctx, cfg.DatabaseURL, migrations.FS); err != nil {
				return d, nil, errs.Wrap("pgpool.Migrate", err)
			}
		}
		pool, err := pgpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return d, nil, errs.Wrap("pgpool.New", err)
		}
		a.pool = pool
		d.Rooms, d.Bans, d.Tx = postgres.NewRoomRepository(pool, cfg.DatabaseReadTimeout), postgres.NewBanRepository(pool, cfg.DatabaseReadTimeout), postgres.NewTxManager(pool)
		a.queue = outbox.NewPostgres(pool, notifyclient.Stream)
		q = a.queue
	} else {
		repo := inmem.NewRepository()
		d.Rooms, d.Bans, d.Tx = repo, repo, &inmem.TxManager{}
		q = outbox.NewMemory()
	}
	if cfg.RedisAddr != "" {
		a.redis = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
		defer cancel()
		if err := a.redis.Ping(ctx).Err(); err != nil {
			return d, nil, errs.Wrap("a.redis.Ping", err)
		}
		state := roomredis.NewState(a.redis, clock{}, cfg.StateTTL)
		d.Presence, d.Index, d.Expiry, d.SweepLock, d.Playback, d.Chat = state, state, state, state, state, state
		a.shared = pubsub.NewRedis(a.redis, "room:events", roomredis.EventCodec{}, 128, a.logger)
		d.Events = a.shared
	} else {
		state := inmem.NewState(clock{}, cfg.StateTTL)
		d.Presence, d.Index, d.Expiry, d.SweepLock, d.Playback, d.Chat = state, state, state, state, state, state
		a.hub = pubsub.New[uuid.UUID, service.Event](128)
		d.Events = a.hub
	}
	return d, q, nil
}
func rateLimits(c config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		roomv1.RoomService_CreateRoom_FullMethodName:  c.CreateLimits,
		roomv1.RoomService_JoinRoom_FullMethodName:    c.JoinLimits,
		roomv1.RoomService_Token_FullMethodName:       c.JoinLimits,
		roomv1.RoomService_SendChat_FullMethodName:    c.ChatLimits,
		roomv1.RoomService_SetPlayback_FullMethodName: c.ScreenLimits,
		roomv1.RoomService_Invite_FullMethodName:      c.InviteLimits,
		roomv1.RoomService_KickMember_FullMethodName:  c.ManageLimits,
		roomv1.RoomService_MuteMember_FullMethodName:  c.ManageLimits,
		roomv1.RoomService_UpdateRoom_FullMethodName:  c.ManageLimits,
		roomv1.RoomService_CloseRoom_FullMethodName:   c.ManageLimits,
	}
}
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return errs.Wrap("a.server.Run", a.server.Run(ctx, a.addr)) })
	if a.metrics != nil {
		g.Go(func() error { return errs.Wrap("a.metrics.Run", a.metrics.Run(ctx)) })
	}
	if a.shared != nil {
		g.Go(func() error { return errs.Wrap("a.shared.Run", a.shared.Run(ctx)) })
	}
	g.Go(func() error { return errs.Wrap("a.relay.Run", a.relay.Run(ctx)) })
	g.Go(func() error { return errs.Wrap("a.mediaRelay.Run", a.mediaRelay.Run(ctx)) })
	g.Go(func() error {
		ticker := time.NewTicker(a.sweepEvery)
		defer ticker.Stop()
		for {
			if err := a.rooms.Sweep(ctx); err != nil && ctx.Err() == nil {
				a.logger.ErrorContext(ctx, "sweep rooms", "error", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	})
	return g.Wait()
}
func Healthcheck(ctx context.Context, cfg config.Config) error {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ClientCredentials", err)
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.ProbeTimeout)
	defer cancel()
	return errs.Wrap("grpcserver.Probe", grpcserver.Probe(ctx, fmt.Sprintf("localhost:%d", cfg.GRPCPort), creds))
}
func (a *App) Close() {
	if a.hub != nil {
		a.hub.Close()
	}
	if a.mediaQueue != nil {
		a.mediaQueue.Close()
	}
	if a.queue != nil {
		a.queue.Close()
	}
	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("limiter close", "error", err)
		}
	}
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Error("redis close", "error", err)
		}
	}
	for _, conn := range a.conns {
		if err := conn.Close(); err != nil {
			a.logger.Error("connection close", "error", err)
		}
	}
	if a.pool != nil {
		a.pool.Close()
	}
	if a.tracer != nil {
		if err := a.tracer.Close(); err != nil {
			a.logger.Error("tracer close", "error", err)
		}
	}
}
