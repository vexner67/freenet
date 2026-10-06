package app

import (
	"context"
	"fmt"
	"log/slog"

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
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc"
)

// maxMessageBytes fits an upload of imaging.MaxUploadBytes with headroom;
// gRPC allows 4 MB by default.
const maxMessageBytes = imaging.MaxUploadBytes + 1<<20

type App struct {
	logger *slog.Logger
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

	if err = a.build(cfg); err != nil {
		return nil, errs.Wrap("a.build", err)
	}

	return a, nil
}

func (a *App) build(cfg config.Config) error {
	store, err := newStore(cfg)
	if err != nil {
		return errs.Wrap("newStore", err)
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
		grpc.MaxRecvMsgSize(maxMessageBytes),
		grpc.ChainUnaryInterceptor(
			mediagrpc.ErrorInterceptor(a.logger),
			jwtauth.UnaryServerInterceptor(
				jwtauth.NewVerifier(publicKey, cfg.JWTIssuer),
				mediagrpc.PublicMethods()...,
			),
		),
	)
	mediav1.RegisterMediaServiceServer(
		a.server.Registrar(),
		mediagrpc.NewMediaHandler(service.NewAvatarService(store), a.logger),
	)

	a.logger.Info("media configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled())

	return nil
}

func newStore(cfg config.Config) (service.AvatarStore, error) {
	if cfg.Storage == config.StorageInmem {
		return inmem.NewAvatarStore(), nil
	}

	store, err := fs.NewAvatarStore(cfg.MediaDir)
	if err != nil {
		return nil, errs.Wrap("fs.NewAvatarStore", err)
	}

	return store, nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.server.Run(ctx, a.addr); err != nil {
		return errs.Wrap("a.server.Run", err)
	}

	return nil
}

// Close is here for symmetry with the other services: media holds no
// connections.
func (a *App) Close() {}
