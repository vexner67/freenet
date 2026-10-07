package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/media/internal/config"
	"github.com/vexner67/freenet/media/internal/fs"
	mediagrpc "github.com/vexner67/freenet/media/internal/grpc"
	"github.com/vexner67/freenet/media/internal/imaging"
	"github.com/vexner67/freenet/media/internal/inmem"
	"github.com/vexner67/freenet/media/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// probeTimeout bounds the healthcheck.
	probeTimeout = 3 * time.Second
	// maxMessageBytes fits an upload of imaging.MaxUploadBytes with
	// headroom; gRPC allows 4 MB by default.
	maxMessageBytes = imaging.MaxUploadBytes + 1<<20

	connectTimeout = 5 * time.Second
)

type App struct {
	logger  *slog.Logger
	limiter ratelimit.Store
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

	if err = a.build(cfg); err != nil {
		return nil, errs.Wrap("a.build", err)
	}

	return a, nil
}

func (a *App) build(cfg config.Config) error {
	stores, err := newStores(cfg)
	if err != nil {
		return errs.Wrap("newStores", err)
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

	reg := metrics.NewRegistry()

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		grpc.MaxRecvMsgSize(maxMessageBytes),
		grpc.ChainUnaryInterceptor(
			mediagrpc.ErrorInterceptor(a.logger),
			jwtauth.UnaryServerInterceptor(
				jwtauth.NewVerifier(publicKey, cfg.JWTIssuer),
				mediagrpc.PublicMethods()...,
			),
			ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger),
		),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}
	mediav1.RegisterMediaServiceServer(
		a.server.Registrar(),
		mediagrpc.NewMediaHandler(
			service.NewAvatarService(stores.avatars),
			service.NewPhotoService(stores.photos),
			a.logger,
		),
	)

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("media configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled())

	return nil
}

// rateLimits are how often each user may upload.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		mediav1.MediaService_UploadPhoto_FullMethodName:  cfg.PhotoLimits,
		mediav1.MediaService_UploadAvatar_FullMethodName: cfg.AvatarLimits,
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

type stores struct {
	avatars service.AvatarStore
	photos  service.PhotoStore
}

func newStores(cfg config.Config) (stores, error) {
	if cfg.Storage == config.StorageInmem {
		return stores{avatars: inmem.NewAvatarStore(), photos: inmem.NewPhotoStore()}, nil
	}

	avatars, err := fs.NewAvatarStore(cfg.MediaDir)
	if err != nil {
		return stores{}, errs.Wrap("fs.NewAvatarStore", err)
	}

	photos, err := fs.NewPhotoStore(cfg.MediaDir)
	if err != nil {
		return stores{}, errs.Wrap("fs.NewPhotoStore", err)
	}

	return stores{avatars: avatars, photos: photos}, nil
}

// Run serves until ctx is cancelled or the gRPC or metrics server fails,
// which stops the other one too.
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

// Close is here for symmetry with the other services: media holds no
// connections.
func (a *App) Close() {
	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
	}
}
