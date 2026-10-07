package grpcserver

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// observer counts calls and logs them at debug level, with the status the
// client got: it runs outside every interceptor a service adds.
type observer struct {
	handled *prometheus.CounterVec
	seconds *prometheus.HistogramVec
	streams *prometheus.GaugeVec
	logger  *slog.Logger
}

func newObserver(reg prometheus.Registerer, logger *slog.Logger) (*observer, error) {
	o := &observer{
		handled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grpc_server_handled_total",
			Help: "RPCs completed on the server, by status code.",
		}, []string{"grpc_type", "grpc_service", "grpc_method", "grpc_code"}),
		seconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "grpc_server_handling_seconds",
			Help:    "How long unary RPCs took on the server.",
			Buckets: prometheus.DefBuckets,
		}, []string{"grpc_service", "grpc_method"}),
		streams: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "grpc_server_streams_open",
			Help: "Server streams open now, such as event subscriptions.",
		}, []string{"grpc_service", "grpc_method"}),
		logger: logger,
	}

	for _, c := range []prometheus.Collector{o.handled, o.seconds, o.streams} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	return o, nil
}

func (o *observer) unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	service, method := splitMethod(info.FullMethod)
	start := time.Now()

	resp, err := handler(ctx, req)

	took := time.Since(start)
	o.seconds.WithLabelValues(service, method).Observe(took.Seconds())
	o.done(ctx, "unary", info.FullMethod, err, took)

	return resp, err
}

func (o *observer) stream(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	service, method := splitMethod(info.FullMethod)
	open := o.streams.WithLabelValues(service, method)
	start := time.Now()

	open.Inc()
	err := handler(srv, ss)
	open.Dec()

	o.done(ss.Context(), "server_stream", info.FullMethod, err, time.Since(start))

	return err
}

func (o *observer) done(ctx context.Context, kind, fullMethod string, err error, took time.Duration) {
	service, method := splitMethod(fullMethod)
	code := status.Code(err)

	o.handled.WithLabelValues(kind, service, method, code.String()).Inc()
	o.logger.DebugContext(ctx, "grpc call",
		"method", fullMethod,
		"code", code.String(),
		"duration_ms", took.Milliseconds(),
	)
}

// splitMethod turns "/user.v1.UserService/GetMe" into its service and
// method.
func splitMethod(fullMethod string) (string, string) {
	service, method, ok := strings.Cut(strings.TrimPrefix(fullMethod, "/"), "/")
	if !ok {
		return "unknown", "unknown"
	}

	return service, method
}
