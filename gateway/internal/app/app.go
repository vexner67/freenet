package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/gateway/internal/backend"
	"github.com/vexner67/freenet/gateway/internal/config"
	"github.com/vexner67/freenet/gateway/internal/preview"
	"github.com/vexner67/freenet/gateway/internal/rest"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcclient"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/tracing"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type App struct {
	logger           *slog.Logger
	tracer           *tracing.Tracer
	authConn         *grpc.ClientConn
	userConn         *grpc.ClientConn
	postConn         *grpc.ClientConn
	mediaConn        *grpc.ClientConn
	notificationConn *grpc.ClientConn
	messageConn      *grpc.ClientConn
	clubConn         *grpc.ClientConn
	server           *rest.Server
	metrics          *metrics.Server
}

// probeTimeout bounds the healthcheck.
const probeTimeout = 3 * time.Second

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
	tracer, err := tracing.New(context.Background(), "gateway", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ClientCredentials", err)
	}

	if !cfg.TLS.Enabled() {
		a.logger.Warn("mTLS is off: calls to internal services are plaintext, do not use in production")
	}

	a.authConn, err = a.dial(cfg.AuthServiceAddr, creds)
	if err != nil {
		return errs.Wrap("a.dial", err)
	}

	a.userConn, err = a.dial(cfg.UserServiceAddr, creds)
	if err != nil {
		return errs.Wrap("a.dial", err)
	}

	a.postConn, err = a.dial(cfg.PostServiceAddr, creds)
	if err != nil {
		return errs.Wrap("a.dial", err)
	}

	a.mediaConn, err = a.dial(cfg.MediaServiceAddr, creds)
	if err != nil {
		return errs.Wrap("a.dial", err)
	}

	a.notificationConn, err = a.dial(cfg.NotificationServiceAddr, creds)
	if err != nil {
		return errs.Wrap("a.dial", err)
	}

	a.messageConn, err = a.dial(cfg.MessageServiceAddr, creds)
	if err != nil {
		return errs.Wrap("a.dial", err)
	}

	a.clubConn, err = a.dial(cfg.ClubServiceAddr, creds)
	if err != nil {
		return errs.Wrap("a.dial", err)
	}

	reg := metrics.NewRegistry()

	a.server, err = rest.NewServer(
		rest.Config{
			Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
			AllowedOrigins:    cfg.CORSAllowedOrigins,
			TrustForwardedFor: cfg.TrustForwardedFor,
			RequestTimeout:    cfg.RequestTimeout,
			StatisticsTTL:     cfg.StatisticsTTL,
			WebDir:            cfg.WebDir,
			Trace:             a.tracer.Handler,
		},
		rest.Backends{
			Auth:          backend.NewAuth(authv1.NewAuthServiceClient(a.authConn)),
			Users:         backend.NewUsers(userv1.NewUserServiceClient(a.userConn)),
			Posts:         backend.NewPosts(postv1.NewPostServiceClient(a.postConn)),
			Media:         backend.NewMedia(mediav1.NewMediaServiceClient(a.mediaConn)),
			Notifications: backend.NewNotifications(notificationv1.NewNotificationServiceClient(a.notificationConn)),
			Messages:      backend.NewMessages(messagev1.NewMessageServiceClient(a.messageConn)),
			Clubs:         backend.NewClubs(clubv1.NewClubServiceClient(a.clubConn)),
			Chats:         backend.NewChats(messagev1.NewMessageServiceClient(a.messageConn)),
			Topics:        backend.NewTopics(postv1.NewPostServiceClient(a.postConn)),
			Albums:        backend.NewAlbums(postv1.NewPostServiceClient(a.postConn)),
			Gifts:         backend.NewGifts(userv1.NewUserServiceClient(a.userConn)),
			Stickers:      backend.NewStickers(userv1.NewUserServiceClient(a.userConn)),
			Documents:     backend.NewDocuments(postv1.NewPostServiceClient(a.postConn)),
			Stories:       backend.NewStories(postv1.NewPostServiceClient(a.postConn)),
			Votes:         backend.NewVotes(userv1.NewUserServiceClient(a.userConn)),
			Ads:           backend.NewAds(userv1.NewUserServiceClient(a.userConn)),
			Decor:         backend.NewDecor(userv1.NewUserServiceClient(a.userConn)),
			Casino:        backend.NewCasino(userv1.NewUserServiceClient(a.userConn)),
			Bets:          backend.NewBets(userv1.NewUserServiceClient(a.userConn)),
			Faves:         backend.NewFaves(postv1.NewPostServiceClient(a.postConn)),
			Previews:      previews(cfg.LinkPreviewTimeout),
		},
		a.logger,
		reg,
	)
	if err != nil {
		return errs.Wrap("rest.NewServer", err)
	}

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("gateway configured", "mtls", cfg.TLS.Enabled(), "cors_origins", cfg.CORSAllowedOrigins, "web_dir", cfg.WebDir)

	return nil
}

// Run serves until ctx is cancelled or the API or metrics server fails,
// which stops the other one too.
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return errs.Wrap("a.server.Run", a.server.Run(ctx))
	})

	if a.metrics != nil {
		g.Go(func() error {
			return errs.Wrap("a.metrics.Run", a.metrics.Run(ctx))
		})
	}

	return g.Wait()
}

// Healthcheck asks the gateway running with cfg on this host whether it
// serves: the container healthcheck, as the image has no shell or curl.
func Healthcheck(ctx context.Context, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	url := fmt.Sprintf("http://localhost:%d%s", cfg.HTTPPort, rest.HealthPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errs.Wrap("http.NewRequestWithContext", err)
	}

	resp, err := (&http.Client{Timeout: probeTimeout}).Do(req)
	if err != nil {
		return errs.Wrap("client.Do", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}

	return nil
}

// dial connects to a service; calls carry the request's ID.
func (a *App) dial(addr string, creds credentials.TransportCredentials) (*grpc.ClientConn, error) {
	return grpcclient.New(addr, creds, a.tracer.DialOption())
}

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	if a.tracer != nil {
		if err := a.tracer.Close(); err != nil {
			a.logger.Error("tracer close", "error", err)
		}
	}

	conns := map[string]*grpc.ClientConn{
		"auth":         a.authConn,
		"user":         a.userConn,
		"post":         a.postConn,
		"media":        a.mediaConn,
		"notification": a.notificationConn,
		"message":      a.messageConn,
		"club":         a.clubConn,
	}

	for name, conn := range conns {
		if conn == nil {
			continue
		}

		if err := conn.Close(); err != nil {
			a.logger.Error("connection close", "service", name, "error", err)
		}
	}
}

// previews reads the cards of links, nil when they are off: a nil
// interface, not a nil *preview.Reader in one.
func previews(timeout time.Duration) rest.Previews {
	if timeout == 0 {
		return nil
	}

	return preview.New(timeout)
}
