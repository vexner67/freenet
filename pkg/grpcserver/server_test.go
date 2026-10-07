package grpcserver_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/reqid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// freeAddr is a local address nobody listens on right now.
func freeAddr(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	return addr
}

// syncBuffer is a log destination the server goroutines write to safely.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type running struct {
	addr string
	reg  *prometheus.Registry
	logs *syncBuffer
	stop context.CancelFunc
	done chan error
	conn *grpc.ClientConn
}

// start runs a server with opts until the test ends and waits until it
// serves.
func start(t *testing.T, opts ...grpc.ServerOption) *running {
	t.Helper()

	r := &running{
		addr: freeAddr(t),
		reg:  prometheus.NewRegistry(),
		logs: &syncBuffer{},
		done: make(chan error, 1),
	}

	logger := slog.New(reqid.LogHandler(slog.NewTextHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	srv, err := grpcserver.New(logger, r.reg, opts...)
	if err != nil {
		t.Fatalf("grpcserver.New: %v", err)
	}

	var ctx context.Context
	ctx, r.stop = context.WithCancel(context.Background())
	go func() { r.done <- srv.Run(ctx, r.addr) }()
	t.Cleanup(func() {
		r.stop()
		<-r.done
	})

	if r.conn, err = grpc.NewClient(r.addr, grpc.WithTransportCredentials(insecure.NewCredentials())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.conn.Close() })

	for range 50 {
		if err = grpcserver.Probe(context.Background(), r.addr, insecure.NewCredentials()); err == nil {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start: %v", err)

	return nil
}

// An open stream, here the endless health Watch, must not hold the server
// up until the graceful stop times out.
func TestServer_StopEndsStreams(t *testing.T) {
	t.Parallel()

	r := start(t)

	watch, err := grpc_health_v1.NewHealthClient(r.conn).Watch(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if _, err = watch.Recv(); err != nil {
		t.Fatalf("Recv: %v", err)
	}

	if got := openStreams(t, r.reg); got != 1 {
		t.Errorf("open streams = %v, want 1", got)
	}

	started := time.Now()
	r.stop()

	if err = <-r.done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	r.done <- nil // for the cleanup
	if took := time.Since(started); took > 2*time.Second {
		t.Fatalf("stopping took %v: the stream held it up", took)
	}

	for {
		if _, err = watch.Recv(); err != nil {
			break // the stream ended
		}
	}

	if got := openStreams(t, r.reg); got != 0 {
		t.Errorf("open streams after stop = %v, want 0", got)
	}
}

// openStreams sums grpc_server_streams_open over its series.
func openStreams(t *testing.T, reg *prometheus.Registry) float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var open float64
	for _, f := range families {
		if f.GetName() == "grpc_server_streams_open" {
			for _, m := range f.GetMetric() {
				open += m.GetGauge().GetValue()
			}
		}
	}

	return open
}

// Calls are counted with the status the client got, after the service's
// own interceptors, and logged with the caller's request ID.
func TestServer_ObservesCalls(t *testing.T) {
	t.Parallel()

	deny := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, _ := metadata.FromIncomingContext(ctx); len(md.Get("deny")) > 0 {
			return nil, status.Error(codes.PermissionDenied, "denied")
		}
		return handler(ctx, req)
	}
	r := start(t, grpc.ChainUnaryInterceptor(deny))
	client := grpc_health_v1.NewHealthClient(r.conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-request-id", "req-42", "deny", "1")
	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Check = %v, want PermissionDenied", err)
	}

	denied := `grpc_server_handled_total{grpc_code="PermissionDenied",grpc_method="Check",grpc_service="grpc.health.v1.Health",grpc_type="unary"} 1`
	if err := testutil.GatherAndCompare(r.reg, strings.NewReader(
		"# HELP grpc_server_handled_total RPCs completed on the server, by status code.\n"+
			"# TYPE grpc_server_handled_total counter\n"+
			`grpc_server_handled_total{grpc_code="OK",grpc_method="Check",grpc_service="grpc.health.v1.Health",grpc_type="unary"} 1`+"\n"+
			denied+"\n",
	), "grpc_server_handled_total"); err != nil {
		t.Error(err) // the OK is start's probe
	}

	if n, err := testutil.GatherAndCount(r.reg, "grpc_server_handling_seconds"); err != nil || n != 1 {
		t.Errorf("handling seconds series = %d (%v), want 1", n, err)
	}

	logs := r.logs.String()
	if !strings.Contains(logs, "code=PermissionDenied") || !strings.Contains(logs, "request_id=req-42") {
		t.Errorf("logs have no call with request_id=req-42:\n%s", logs)
	}
}

func TestNew_RegistersOnce(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	logger := slog.New(slog.DiscardHandler)

	if _, err := grpcserver.New(logger, reg); err != nil {
		t.Fatalf("first New: %v", err)
	}
	if _, err := grpcserver.New(logger, reg); err == nil {
		t.Error("second New on the same registry = nil, want an error")
	}
}

func TestProbe_NotServing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := grpcserver.Probe(ctx, freeAddr(t), insecure.NewCredentials()); err == nil {
		t.Error("Probe of a closed port = nil, want an error")
	}
}
