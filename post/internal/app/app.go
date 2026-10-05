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
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/config"
	postgrpc "github.com/vexner67/freenet/post/internal/grpc"
	"github.com/vexner67/freenet/post/internal/inmem"
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
	logger *slog.Logger
	pool   *pgxpool.Pool
	server *grpcserver.Server
	addr   string
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
		),
	)
	postv1.RegisterPostServiceServer(
		a.server.Registrar(),
		postgrpc.NewPostHandler(
			service.NewPostService(repos.posts, repos.likes),
			service.NewCommentService(repos.posts, repos.comments),
			a.logger,
		),
	)

	a.logger.Info("post configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled())

	return nil
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
	if a.pool != nil {
		a.pool.Close()
	}
}
