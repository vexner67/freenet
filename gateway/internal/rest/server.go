package rest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const shutdownTimeout = 5 * time.Second

type Config struct {
	Addr              string
	AllowedOrigins    []string
	TrustForwardedFor bool
	RequestTimeout    time.Duration
	// WebDir is the website to serve next to the API; empty serves API only.
	WebDir string
}

type Server struct {
	http   *http.Server
	logger *slog.Logger
}

func NewServer(cfg Config, backends Backends, logger *slog.Logger) *Server {
	a := &api{
		auth:              backends.Auth,
		users:             backends.Users,
		posts:             backends.Posts,
		media:             backends.Media,
		notifications:     backends.Notifications,
		messages:          backends.Messages,
		logger:            logger,
		trustForwardedFor: cfg.TrustForwardedFor,
		webDir:            cfg.WebDir,
	}

	handler := chain(
		a.routes(),
		a.recoverPanics,
		logRequests(logger),
		secureHeaders,
		cors(cfg.AllowedOrigins),
		timeout(cfg.RequestTimeout),
	)

	return &Server{
		http: &http.Server{
			Addr:              cfg.Addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			// Long enough to upload a photo over a slow connection.
			ReadTimeout:  time.Minute,
			WriteTimeout: cfg.RequestTimeout + 5*time.Second,
			IdleTimeout:  time.Minute,
		},
		logger: logger,
	}
}

// Handler is the whole API with its middleware, for tests.
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

// Run serves until ctx is cancelled, then finishes in-flight requests.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.http.ListenAndServe()
	}()

	s.logger.Info("HTTP server started", "address", s.http.Addr)

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("s.http.ListenAndServe: %w", err)
		}
		return nil
	case <-ctx.Done():
		s.logger.Info("stopping HTTP server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("s.http.Shutdown: %w", err)
	}

	s.logger.Info("HTTP server stopped")

	return nil
}
