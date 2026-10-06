package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/ratelimit"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/config"
	postgrpc "github.com/vexner67/freenet/post/internal/grpc"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/notifyclient"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
	"github.com/vexner67/freenet/post/migrations"
	"google.golang.org/grpc"
)

const (
	connectTimeout = 5 * time.Second
	migrateTimeout = time.Minute
)

type App struct {
	logger           *slog.Logger
	pool             *pgxpool.Pool
	limiter          ratelimit.Store
	notificationConn *grpc.ClientConn
	server           *grpcserver.Server
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
	repos, err := a.newRepositories(cfg)
	if err != nil {
		return errs.Wrap("a.newRepositories", err)
	}

	notifier, err := a.newNotifier(cfg)
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

	a.server = grpcserver.New(
		a.logger,
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
	postv1.RegisterPostServiceServer(
		a.server.Registrar(),
		postgrpc.NewPostHandler(
			service.NewPostService(repos.posts, repos.likes, notifier),
			service.NewCommentService(repos.posts, repos.comments, notifier),
			a.logger,
		),
	)

	a.logger.Info("post configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(),
		"notifications", cfg.NotificationServiceAddr != "")

	return nil
}

// rateLimits are what each user may write how often.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		postv1.PostService_CreatePost_FullMethodName:    cfg.PostLimits,
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

// newNotifier connects to the notification service; the connection is
// lazy, so the services may start in any order.
func (a *App) newNotifier(cfg config.Config) (service.Notifier, error) {
	if cfg.NotificationServiceAddr == "" {
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: likes and comments notify nobody")
		return notifyclient.Nop{}, nil
	}

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	a.notificationConn, err = grpc.NewClient(cfg.NotificationServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, errs.Wrap("grpc.NewClient", err)
	}

	return notifyclient.New(notificationv1.NewNotificationServiceClient(a.notificationConn), cfg.NotifyTimeout, a.logger), nil
}

type repositories struct {
	posts    service.PostRepository
	likes    service.LikeRepository
	comments service.CommentRepository
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		return repositories{posts: store.Posts(), likes: store.Likes(), comments: store.Comments()}, nil
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
		posts:    postgres.NewPostRepository(a.pool),
		likes:    postgres.NewLikeRepository(a.pool),
		comments: postgres.NewCommentRepository(a.pool),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.server.Run(ctx, a.addr); err != nil {
		return errs.Wrap("a.server.Run", err)
	}

	return nil
}

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
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
