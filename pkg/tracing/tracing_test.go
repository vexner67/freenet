package tracing

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vexner67/freenet/pkg/reqid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func recording(t *testing.T, service string, ratio float64) (*Tracer, *tracetest.InMemoryExporter) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tracer := newTracer(service, ratio, sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tracer.Close() })

	return tracer, exporter
}

// serve starts a server that answers any method with an empty message,
// and the health service.
func serve(t *testing.T, tracer *Tracer) *grpc.ClientConn {
	t.Helper()

	echo := func(_ any, stream grpc.ServerStream) error {
		var in emptypb.Empty
		if err := stream.RecvMsg(&in); err != nil {
			return err
		}

		return stream.SendMsg(&emptypb.Empty{})
	}

	srv := grpc.NewServer(tracer.ServerOption(), grpc.UnknownServiceHandler(echo))
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return dial(t, lis.Addr().String(), tracer)
}

func dial(t *testing.T, addr string, tracer *Tracer) *grpc.ClientConn {
	t.Helper()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), tracer.DialOption())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func TestTracer_RequestThroughServices(t *testing.T) {
	t.Parallel()

	tracer, exporter := recording(t, "gateway", 1)
	conn := serve(t, tracer)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/users/{nickname}", func(w http.ResponseWriter, r *http.Request) {
		if err := conn.Invoke(r.Context(), "/user.v1.UserService/GetProfile", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
			t.Errorf("Invoke: %v", err)
		}

		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/ann", nil)
	// A trace the client names is not ours to continue.
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	req = req.WithContext(reqid.ContextWithID(req.Context(), "req-1"))

	tracer.Handler(mux).ServeHTTP(httptest.NewRecorder(), req)

	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("got %d spans, want the request, the call and the call served: %v", len(spans), names(spans))
	}

	// Ended in this order: served, called, requested.
	served, called, root := spans[0], spans[1], spans[2]

	if root.Name != "GET /api/v1/users/{nickname}" {
		t.Errorf("root span is %q, want the route", root.Name)
	}
	if root.Parent.IsValid() || root.SpanContext.TraceID().String() == "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("root span continues the trace of the client")
	}
	if got := attr(root, reqid.LogKey); got != "req-1" {
		t.Errorf("request_id = %q, want req-1", got)
	}
	if got := attr(root, "http.response.status_code"); got != "418" {
		t.Errorf("status = %q, want 418", got)
	}

	if called.SpanKind != trace.SpanKindClient || called.Parent.SpanID() != root.SpanContext.SpanID() {
		t.Errorf("the call is not a child of the request: %+v", called)
	}
	if served.SpanKind != trace.SpanKindServer || served.Parent.SpanID() != called.SpanContext.SpanID() {
		t.Errorf("the call served is not a child of the call: %+v", served)
	}
	if served.SpanContext.TraceID() != root.SpanContext.TraceID() {
		t.Errorf("the call served is in another trace")
	}
}

func TestTracer_HealthChecksLeftOut(t *testing.T) {
	t.Parallel()

	tracer, exporter := recording(t, "user", 1)
	conn := serve(t, tracer)

	// The probe of a healthcheck has no tracer of its own.
	probe := dial(t, conn.Target(), &Tracer{})

	if _, err := grpc_health_v1.NewHealthClient(probe).Check(context.Background(), &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatalf("Check: %v", err)
	}

	if spans := exporter.GetSpans(); len(spans) != 0 {
		t.Fatalf("a health check left spans: %v", names(spans))
	}
}

func TestTracer_SampleRatio(t *testing.T) {
	t.Parallel()

	tracer, exporter := recording(t, "gateway", 0)

	tracer.Handler(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if spans := exporter.GetSpans(); len(spans) != 0 {
		t.Fatalf("a ratio of 0 left spans: %v", names(spans))
	}
}

func TestTracer_Off(t *testing.T) {
	t.Parallel()

	tracer, err := New(context.Background(), "gateway", Config{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	called := false
	handler := tracer.Handler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true

		if trace.SpanContextFromContext(r.Context()).IsValid() {
			t.Error("a span without an endpoint")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatal("the handler was not called")
	}

	conn := serve(t, tracer)
	if err := conn.Invoke(context.Background(), "/user.v1.UserService/GetProfile", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if err := tracer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	for ratio, ok := range map[float64]bool{0: true, 0.25: true, 1: true, -0.1: false, 1.5: false} {
		if err := (Config{SampleRatio: ratio}).Validate(); (err == nil) != ok {
			t.Errorf("ratio %v: %v", ratio, err)
		}
	}
}

func names(spans tracetest.SpanStubs) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name)
	}

	return out
}

func attr(span tracetest.SpanStub, key string) string {
	for _, kv := range span.Attributes {
		if string(kv.Key) == key {
			return kv.Value.String()
		}
	}

	return ""
}
