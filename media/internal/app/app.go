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
	"github.com/vexner67/freenet/media/internal/s3"
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
	logger       *slog.Logger
	limiter      ratelimit.Store
	server       *grpcserver.Server
	metrics      *metrics.Server
	photos       *service.PhotoService
	cleanupEvery time.Duration
	addr         string
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
		logger:       log,
		cleanupEvery: cfg.PhotoCleanupInterval,
		addr:         fmt.Sprintf(":%d", cfg.GRPCPort),
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
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	reg := metrics.NewRegistry()

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		grpc.MaxRecvMsgSize(maxMessageBytes),
		grpc.ChainUnaryInterceptor(
			mediagrpc.ErrorInterceptor(a.logger),
			restrictInternal(cfg),
			jwtauth.UnaryServerInterceptor(
				jwtauth.NewVerifier(publicKey, cfg.JWTIssuer),
				jwtauth.Access{Public: mediagrpc.PublicMethods()},
			),
			ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger),
		),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}
	a.photos = service.NewPhotoService(stores.photos, cfg.UnpublishedPhotoTTL, cfg.RetiredPhotoTTL)

	mediav1.RegisterMediaServiceServer(
		a.server.Registrar(),
		mediagrpc.NewMediaHandler(
			service.NewAvatarService(stores.avatars),
			a.photos,
			service.NewPurgeService(stores.avatars, stores.photos),
			a.logger,
		),
	)

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("media configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(),
		"unpublished_photo_ttl", cfg.UnpublishedPhotoTTL, "retired_photo_ttl", cfg.RetiredPhotoTTL)

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

	return mtls.RestrictMethods(mediagrpc.InternalMethods())
}

// rateLimits are how often each user may upload.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		mediav1.MediaService_UploadPhoto_FullMethodName:  cfg.PhotoLimits,
		mediav1.MediaService_UploadAvatar_FullMethodName: cfg.AvatarLimits,
		// Counted by the caller, the admin who uploads.
		mediav1.MediaService_UploadClubAvatar_FullMethodName: cfg.AvatarLimits,
		// A backdrop is as rare as a photo of the page, with its own count.
		mediav1.MediaService_UploadClubPhoto_FullMethodName: cfg.AvatarLimits,
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
	switch cfg.Storage {
	case config.StorageInmem:
		return stores{avatars: inmem.NewAvatarStore(), photos: inmem.NewPhotoStore()}, nil
	case config.StorageS3:
		return newBucketStores(cfg)
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

func newBucketStores(cfg config.Config) (stores, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	bucket, err := s3.Open(ctx, s3.Config{
		Endpoint:  cfg.S3Endpoint,
		Region:    cfg.S3Region,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Prefix:    cfg.S3Prefix,
		Insecure:  cfg.S3Insecure,
	})
	if err != nil {
		return stores{}, errs.Wrap("s3.Open", err)
	}

	return stores{avatars: s3.NewAvatarStore(bucket), photos: s3.NewPhotoStore(bucket)}, nil
}

// Run serves and deletes unpublished photos until ctx is cancelled or the
// gRPC or metrics server fails, which stops the rest too.
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
		a.runCleanup(ctx)
		return nil
	})

	return g.Wait()
}

// runCleanup deletes the photos that never got their post, every
// cleanupEvery until ctx ends; a failure is logged and tried next time.
func (a *App) runCleanup(ctx context.Context) {
	t := time.NewTicker(a.cleanupEvery)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		deleted, err := a.photos.DeleteUnpublished(ctx)
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "delete unpublished photos", "error", err)
		}
		if deleted > 0 {
			a.logger.InfoContext(ctx, "unpublished photos deleted", "count", deleted)
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

// Close closes the rate limiter: the stores hold no connections of their
// own.
func (a *App) Close() {
	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
	}
}
