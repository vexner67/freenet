package reqid_test

import (
	"context"
	"net"
	"testing"

	"github.com/vexner67/freenet/pkg/reqid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

// seen is the ID the server saw on its last call.
type seen struct {
	ids chan string
}

func (s seen) unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	s.ids <- reqid.FromContext(ctx)
	return handler(ctx, req)
}

func (s seen) stream(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	s.ids <- reqid.FromContext(ss.Context())
	return nil
}

func dial(t *testing.T, s seen) grpc_health_v1.HealthClient {
	t.Helper()

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(reqid.UnaryServerInterceptor, s.unary),
		grpc.ChainStreamInterceptor(reqid.StreamServerInterceptor, s.stream),
	)
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		append(reqid.DialOptions(),
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)...,
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return grpc_health_v1.NewHealthClient(conn)
}

func TestGRPC_PassesID(t *testing.T) {
	t.Parallel()

	s := seen{ids: make(chan string, 1)}
	client := dial(t, s)
	ctx := reqid.ContextWithID(t.Context(), "from-gateway")

	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := <-s.ids; got != "from-gateway" {
		t.Errorf("unary call: server saw %q, want from-gateway", got)
	}

	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	_, _ = stream.Recv()
	if got := <-s.ids; got != "from-gateway" {
		t.Errorf("stream: server saw %q, want from-gateway", got)
	}
}

func TestGRPC_MakesIDWhenMissingOrInvalid(t *testing.T) {
	t.Parallel()

	for name, ctx := range map[string]context.Context{
		"missing": t.Context(),
		"invalid": reqid.ContextWithID(t.Context(), "bad\nid"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := seen{ids: make(chan string, 1)}
			if _, err := dial(t, s).Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
				t.Fatalf("Check: %v", err)
			}

			if got := <-s.ids; !reqid.Valid(got) {
				t.Errorf("server saw %q, want a new valid ID", got)
			}
		})
	}
}
