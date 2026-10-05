package app

import (
	"context"
	"fmt"
	"log/slog"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/gateway/internal/backend"
	"github.com/vexner67/freenet/gateway/internal/config"
	"github.com/vexner67/freenet/gateway/internal/rest"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/mtls"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
)

type App struct {
	logger    *slog.Logger
	authConn  *grpc.ClientConn
	userConn  *grpc.ClientConn
	postConn  *grpc.ClientConn
	mediaConn *grpc.ClientConn
	server    *rest.Server
}

func New(cfg config.Config) (*App, error) {
	log, err := logger.New(logger.Config{
		LogLevel:  cfg.LogLevel,
		LogFormat: cfg.LogFormat,
	})
	if err != nil {
		return nil, errs.Wrap("logger.New", err)
	}

	a := &App{logger: log}

	// On failure Close releases whatever was opened before it.
	if err = a.build(cfg); err != nil {
		a.Close()
		return nil, errs.Wrap("a.build", err)
	}

	return a, nil
}

func (a *App) build(cfg config.Config) error {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ClientCredentials", err)
	}

	if !cfg.TLS.Enabled() {
		a.logger.Warn("mTLS is off: calls to internal services are plaintext, do not use in production")
	}

	a.authConn, err = grpc.NewClient(cfg.AuthServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return errs.Wrap("grpc.NewClient", err)
	}

	a.userConn, err = grpc.NewClient(cfg.UserServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return errs.Wrap("grpc.NewClient", err)
	}

	a.postConn, err = grpc.NewClient(cfg.PostServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return errs.Wrap("grpc.NewClient", err)
	}

	a.mediaConn, err = grpc.NewClient(cfg.MediaServiceAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return errs.Wrap("grpc.NewClient", err)
	}

	a.server = rest.NewServer(
		rest.Config{
			Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
			AllowedOrigins:    cfg.CORSAllowedOrigins,
			TrustForwardedFor: cfg.TrustForwardedFor,
			RequestTimeout:    cfg.RequestTimeout,
			WebDir:            cfg.WebDir,
		},
		backend.NewAuth(authv1.NewAuthServiceClient(a.authConn)),
		backend.NewUsers(userv1.NewUserServiceClient(a.userConn)),
		backend.NewPosts(postv1.NewPostServiceClient(a.postConn)),
		backend.NewMedia(mediav1.NewMediaServiceClient(a.mediaConn)),
		a.logger,
	)

	a.logger.Info("gateway configured", "mtls", cfg.TLS.Enabled(), "cors_origins", cfg.CORSAllowedOrigins, "web_dir", cfg.WebDir)

	return nil
}

func (a *App) Run(ctx context.Context) error {
	if err := a.server.Run(ctx); err != nil {
		return errs.Wrap("a.server.Run", err)
	}

	return nil
}

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	for name, conn := range map[string]*grpc.ClientConn{"auth": a.authConn, "user": a.userConn, "post": a.postConn, "media": a.mediaConn} {
		if conn == nil {
			continue
		}

		if err := conn.Close(); err != nil {
			a.logger.Error("connection close", "service", name, "error", err)
		}
	}
}
