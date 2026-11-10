package grpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"github.com/vexner67/freenet/pkg/mtls"
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
	roomgrpc "github.com/vexner67/freenet/room/internal/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func identity(name string) context.Context {
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: name}}
	return peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}}}})
}
func TestInternalMethodsAlwaysRequireMTLS(t *testing.T) {
	t.Parallel()
	auth := roomgrpc.AuthInterceptor(nil)
	for method, allowed := range roomgrpc.InternalMethods() {
		for _, name := range []string{"", mtls.Auth, mtls.Gateway, mtls.User, mtls.Club, mtls.Message} {
			ctx := context.Background()
			if name != "" {
				ctx = identity(name)
			}
			called := false
			_, err := auth(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) { called = true; return "ok", nil })
			want := false
			for _, service := range allowed {
				want = want || service == name
			}
			if called != want || (!want && status.Code(err) != codes.PermissionDenied) || (want && err != nil) {
				t.Fatalf("%s from %q: called=%v err=%v", method, name, called, err)
			}
		}
	}
}
func TestOnlyReadsArePublic(t *testing.T) {
	t.Parallel()
	public := map[string]bool{}
	for _, method := range roomgrpc.PublicMethods() {
		public[method] = true
	}
	for method := range roomgrpc.InternalMethods() {
		if public[method] {
			t.Fatalf("internal method is public: %s", method)
		}
	}
	if len(public) != 3 {
		t.Fatal(public)
	}
	auth := roomgrpc.AuthInterceptor(nil)
	for _, method := range []string{roomv1.RoomService_CreateRoom_FullMethodName, roomv1.RoomService_JoinRoom_FullMethodName, roomv1.RoomService_SendChat_FullMethodName} {
		_, err := auth(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) { t.Fatal("anonymous mutation"); return nil, nil })
		if status.Code(err) != codes.Unauthenticated {
			t.Fatal(err)
		}
	}
}
