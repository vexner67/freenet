// Package tracing follows a request through the services: an HTTP request
// of the gateway and every gRPC call it leads to are spans of one trace,
// sent to a collector that speaks OTLP (Jaeger in the Docker stack). The
// root span carries the request ID, which the log lines of the request
// carry too.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/vexner67/freenet/pkg/reqid"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc/filters"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// Config is embedded by services with envPrefix "TRACING_":
// TRACING_ENDPOINT and TRACING_SAMPLE_RATIO.
type Config struct {
	// Endpoint is the host:port of the collector that takes OTLP over
	// gRPC, in plaintext: it is reached inside the internal network only.
	// Without it nothing is traced.
	Endpoint string `env:"ENDPOINT"`
	// SampleRatio is the share of requests traced, from 0 to 1; a call
	// that came with a trace follows what its caller decided.
	SampleRatio float64 `env:"SAMPLE_RATIO" envDefault:"1"`
}

func (c Config) Enabled() bool {
	return c.Endpoint != ""
}

func (c Config) Validate() error {
	if c.SampleRatio < 0 || c.SampleRatio > 1 {
		return errors.New("TRACING_SAMPLE_RATIO must be from 0 to 1")
	}

	return nil
}

// Tracer makes the spans of one service. Without an endpoint it makes
// none and costs nothing.
type Tracer struct {
	provider   *sdktrace.TracerProvider
	propagator propagation.TextMapPropagator
}

// New starts sending the spans of the service to the collector of cfg. A
// collector that is down loses spans, with a warning in the log, and
// holds nothing back.
func New(ctx context.Context, service string, cfg Config, logger *slog.Logger) (*Tracer, error) {
	if !cfg.Enabled() {
		return &Tracer{}, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otlptracegrpc.New: %w", err)
	}

	// The SDK reports spans it could not send here, or to stderr.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("tracing", "error", err)
	}))

	return newTracer(service, cfg.SampleRatio, sdktrace.WithBatcher(exporter)), nil
}

func newTracer(service string, ratio float64, processor sdktrace.TracerProviderOption) *Tracer {
	return &Tracer{
		provider: sdktrace.NewTracerProvider(
			processor,
			sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		),
		propagator: propagation.TraceContext{},
	}
}

// flushTimeout bounds Close: a collector that is down must not hold a
// stopping service.
const flushTimeout = 5 * time.Second

// Close sends the spans not sent yet.
func (t *Tracer) Close() error {
	if t.provider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()

	if err := t.provider.Shutdown(ctx); err != nil {
		return fmt.Errorf("t.provider.Shutdown: %w", err)
	}

	return nil
}

// ServerOption makes every call the gRPC server takes a span, a child of
// the caller's. Health checks are left out: there is one every few
// seconds.
func (t *Tracer) ServerOption() grpc.ServerOption {
	if t.provider == nil {
		return grpc.EmptyServerOption{}
	}

	return grpc.StatsHandler(otelgrpc.NewServerHandler(append(t.options(),
		otelgrpc.WithFilter(filters.Not(filters.HealthCheck())))...))
}

// DialOption makes every call to another service a span and tells the
// service which trace it belongs to.
func (t *Tracer) DialOption() grpc.DialOption {
	if t.provider == nil {
		return grpc.EmptyDialOption{}
	}

	return grpc.WithStatsHandler(otelgrpc.NewClientHandler(t.options()...))
}

func (t *Tracer) options() []otelgrpc.Option {
	return []otelgrpc.Option{
		otelgrpc.WithTracerProvider(t.provider),
		otelgrpc.WithPropagators(t.propagator),
		// Calls are counted by pkg/grpcserver already.
		otelgrpc.WithMeterProvider(noop.NewMeterProvider()),
	}
}

// Handler makes every HTTP request the root span of a new trace, named by
// the route that served it — never by the path, which holds nicknames and
// IDs — and carrying the request ID. Whatever trace the client names is
// ignored. It must reach the mux through handlers that pass the request
// on as it is: only the request the mux gets learns its pattern.
func (t *Tracer) Handler(next http.Handler) http.Handler {
	if t.provider == nil {
		return next
	}

	tracer := t.provider.Tracer("github.com/vexner67/freenet/pkg/tracing")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), r.Method,
			trace.WithNewRoot(),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String(reqid.LogKey, reqid.FromContext(r.Context()))),
		)
		defer span.End()

		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		r = r.WithContext(ctx)

		next.ServeHTTP(rec, r)

		if r.Pattern != "" {
			span.SetName(r.Pattern)
		}

		span.SetAttributes(attribute.Int("http.response.status_code", rec.status))

		if rec.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		}
	})
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController and the WebSocket upgrade reach the
// connection underneath.
func (r *recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
