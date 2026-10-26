package rest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const shutdownTimeout = 5 * time.Second

type Config struct {
	Addr              string
	AllowedOrigins    []string
	TrustForwardedFor bool
	RequestTimeout    time.Duration
	// StatisticsTTL is how long the numbers of "о сайте" are kept before
	// the services count again; zero counts on every request.
	StatisticsTTL time.Duration
	// WebDir is the website to serve next to the API; empty serves API only.
	WebDir string
	// Trace makes every request the root span of a trace
	// (tracing.Tracer.Handler); nil traces nothing.
	Trace func(http.Handler) http.Handler
}

// HealthPath answers 200 while the server serves.
const HealthPath = "/healthz"

type Server struct {
	http   *http.Server
	logger *slog.Logger
	// streams are the event streams still open; Shutdown does not wait
	// for hijacked connections, so Run does.
	streams *sync.WaitGroup
}

// NewServer registers the HTTP metrics in reg.
func NewServer(cfg Config, backends Backends, logger *slog.Logger, reg prometheus.Registerer) (*Server, error) {
	metrics, err := newHTTPMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("newHTTPMetrics: %w", err)
	}

	stopping, stop := context.WithCancel(context.Background())

	a := &api{
		auth:              backends.Auth,
		users:             backends.Users,
		posts:             backends.Posts,
		topics:            backends.Topics,
		albums:            backends.Albums,
		faves:             backends.Faves,
		gifts:             backends.Gifts,
		stickers:          backends.Stickers,
		documents:         backends.Documents,
		stories:           backends.Stories,
		previews:          backends.Previews,
		votes:             backends.Votes,
		ads:               backends.Ads,
		decors:            backends.Decor,
		primes:            backends.Prime,
		casinos:           backends.Casino,
		mines:             backends.Mine,
		firmsBackend:      backends.Firms,
		titles:            backends.Titles,
		banks:             backends.Bank,
		pokers:            backends.Poker,
		holdems:           backends.Holdem,
		pets:              backends.Pets,
		bets:              backends.Bets,
		media:             backends.Media,
		notifications:     backends.Notifications,
		messages:          backends.Messages,
		clubs:             backends.Clubs,
		chats:             backends.Chats,
		logger:            logger,
		trustForwardedFor: cfg.TrustForwardedFor,
		webDir:            cfg.WebDir,
		eventOrigins:      eventOrigins(cfg.AllowedOrigins),
		eventStreams:      metrics.events,
		stopping:          stopping,
		statistics:        &statistics{ttl: cfg.StatisticsTTL},
	}

	mux := a.routes()
	a.batched = metrics.observe(mux)

	handler := chain(
		mux,
		requestID(cfg.TrustForwardedFor),
		a.recoverPanics,
		logRequests(logger),
		secureHeaders,
		noIndex,
		cors(cfg.AllowedOrigins),
		timeout(cfg.RequestTimeout),
		// Next to the mux, as observe: it names the span by the route.
		traced(cfg.Trace),
		metrics.observe,
	)

	srv := &Server{
		http: &http.Server{
			Addr:              cfg.Addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			// Long enough to upload a photo over a slow connection.
			ReadTimeout:  time.Minute,
			WriteTimeout: cfg.RequestTimeout + 5*time.Second,
			IdleTimeout:  time.Minute,
		},
		logger:  logger,
		streams: &a.streams,
	}
	srv.http.RegisterOnShutdown(stop)

	return srv, nil
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

	// The event streams got "going away" as Shutdown began; let them close.
	closed := make(chan struct{})
	go func() {
		s.streams.Wait()
		close(closed)
	}()

	select {
	case <-closed:
	case <-shutdownCtx.Done():
		s.logger.Warn("event streams still open after shutdown")
	}

	s.logger.Info("HTTP server stopped")

	return nil
}
