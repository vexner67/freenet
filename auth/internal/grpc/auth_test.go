package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"testing"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// callFrom builds the context of a call from the given mTLS identity
// ("" = no TLS) that claims the client address 198.51.100.7.
func callFrom(identity string, claimed string) context.Context {
	p := &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 41000}}

	if identity != "" {
		cert := &x509.Certificate{Subject: pkix.Name{CommonName: identity}}
		p.AuthInfo = credentials.TLSInfo{State: tls.ConnectionState{
			VerifiedChains: [][]*x509.Certificate{{cert}},
		}}
	}

	ctx := peer.NewContext(context.Background(), p)

	return metadata.NewIncomingContext(ctx, metadata.Pairs(authv1.ClientIPMetadata, claimed))
}

func TestClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity string
		claimed  string
		want     string
	}{
		{"gateway states the client address", mtls.Gateway, "198.51.100.7", "198.51.100.7"},
		{"gateway with ipv6", mtls.Gateway, "2001:db8::1", "2001:db8::1"},
		{"gateway with garbage falls back to connection", mtls.Gateway, "not-an-ip", "10.0.0.2"},
		{"another service cannot state it", mtls.User, "198.51.100.7", "10.0.0.2"},
		{"without mTLS metadata is ignored", "", "198.51.100.7", "10.0.0.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := clientIP(callFrom(tt.identity, tt.claimed)); got != tt.want {
				t.Fatalf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}
