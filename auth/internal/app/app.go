package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/auth/internal/config"
	"github.com/vexner67/freenet/auth/internal/console"
	authgrpc "github.com/vexner67/freenet/auth/internal/grpc"
	"github.com/vexner67/freenet/auth/internal/inmem"
	"github.com/vexner67/freenet/auth/internal/jwt"
	"github.com/vexner67/freenet/auth/internal/postgres"
	"github.com/vexner67/freenet/auth/internal/redis"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/smtp"
	"github.com/vexner67/freenet/auth/internal/userclient"
	"github.com/vexner67/freenet/auth/migrations"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/hmac"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/ratelimit"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
)

const (
	connectTimeout = 5 * time.Second
	migrateTimeout = time.Minute
)

type App struct {
	logger   *slog.Logger
	pool     *pgxpool.Pool
	redis    *goredis.Client
	userConn *grpc.ClientConn
	server   *grpcserver.Server
	addr     string
}

// storage is the set of repositories chosen by STORAGE.
type storage struct {
	sessions service.SessionRepository
	codes    service.CodeRepository
	limiter  service.RateLimiter
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
	store, err := a.newStorage(cfg)
	if err != nil {
		return errs.Wrap("a.newStorage", err)
	}

	clientCreds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ClientCredentials", err)
	}

	a.userConn, err = grpc.NewClient(cfg.UserServiceAddr, grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		return errs.Wrap("grpc.NewClient", err)
	}

	tokenProvider, err := newTokenProvider(cfg)
	if err != nil {
		return errs.Wrap("newTokenProvider", err)
	}

	authService := service.NewAuthService(
		userclient.New(userv1.NewUserServiceClient(a.userConn)),
		store.sessions,
		hmac.NewHasher([]byte(cfg.HashSecret)),
		store.codes,
		store.limiter,
		a.newMailer(cfg),
		tokenProvider,
		service.Policy{
			CodeTTL:            cfg.Code.TTL,
			MaxCodeAttempts:    cfg.Code.MaxAttempts,
			CodeResendInterval: cfg.Code.ResendInterval,
			CodeRequestsPerIP:  cfg.Code.RequestsPerIP,
			CodeRequestsWindow: cfg.Code.RequestsWindow,
		},
	)

	serverCreds, err := mtls.ServerCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ServerCredentials", err)
	}

	if !cfg.TLS.Enabled() {
		a.logger.Warn("mTLS is off: client addresses from the gateway are not trusted, do not use in production")
	}

	a.server = grpcserver.New(
		a.logger,
		grpc.Creds(serverCreds),
		grpc.ChainUnaryInterceptor(authgrpc.ErrorInterceptor(a.logger)),
	)
	authv1.RegisterAuthServiceServer(a.server.Registrar(), authgrpc.NewAuthHandler(authService, a.logger))

	a.logger.Info("auth configured", "storage", cfg.Storage, "mailer", cfg.Mailer, "mtls", cfg.TLS.Enabled())

	return nil
}

func (a *App) newStorage(cfg config.Config) (storage, error) {
	if cfg.Storage == config.StorageInmem {
		return storage{
			sessions: inmem.NewSessionRepository(),
			codes:    inmem.NewCodeRepository(),
			limiter:  ratelimit.NewMemory(),
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

	a.redis, err = redis.New(ctx, cfg.RedisAddr)
	if err != nil {
		return storage{}, errs.Wrap("redis.New", err)
	}

	return storage{
		sessions: postgres.NewSessionRepository(a.pool),
		codes:    redis.NewCodeRepository(a.redis),
		limiter:  ratelimit.NewRedis(a.redis),
	}, nil
}

func (a *App) newMailer(cfg config.Config) service.Mailer {
	if cfg.Mailer == config.MailerSMTP {
		return smtp.NewMailer(
			cfg.SMTP.Host,
			cfg.SMTP.Port,
			cfg.SMTP.Username,
			cfg.SMTP.Password,
			cfg.SMTP.From,
		)
	}

	a.logger.Warn("verification codes are written to the log (MAILER=console), do not use in production")

	return console.NewMailer(a.logger)
}

func newTokenProvider(cfg config.Config) (*jwt.JWT, error) {
	privateKey, err := config.LoadPrivateKey(cfg.JWTPrivateKeyPath)
	if err != nil {
		return nil, errs.Wrap("config.LoadPrivateKey", err)
	}

	publicKey, err := jwtauth.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, errs.Wrap("jwtauth.LoadPublicKey", err)
	}

	return jwt.New(jwt.Config{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Issuer:     cfg.Issuer,
		AccessTTL:  cfg.AccessTokenTTL,
		RefreshTTL: cfg.RefreshTokenTTL,
	}), nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.server.Run(ctx, a.addr); err != nil {
		return errs.Wrap("a.server.Run", err)
	}

	return nil
}

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	if a.userConn != nil {
		if err := a.userConn.Close(); err != nil {
			a.logger.Error("user connection close", "error", err)
		}
	}

	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Error("redis close", "error", err)
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
