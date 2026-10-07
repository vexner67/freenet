// Package metrics exposes a service's Prometheus metrics on a port of its
// own, kept off the public one: what it counts is for operators only.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// Path is where the metrics are served.
	Path            = "/metrics"
	scrapeTimeout   = 10 * time.Second
	shutdownTimeout = 5 * time.Second
)

// NewRegistry returns a registry with the Go runtime and process metrics;
// services add their own to it.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return reg
}

type Server struct {
	http   *http.Server
	logger *slog.Logger
}

func NewServer(addr string, gatherer prometheus.Gatherer, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	mux.Handle("GET "+Path, promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		Timeout:  scrapeTimeout,
	}))

	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      scrapeTimeout + 5*time.Second,
		},
		logger: logger,
	}
}

// Run serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.http.ListenAndServe()
	}()

	s.logger.Info("metrics server started", "address", s.http.Addr)

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("s.http.ListenAndServe: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("s.http.Shutdown: %w", err)
	}

	return nil
}
