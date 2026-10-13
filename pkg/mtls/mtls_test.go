package mtls_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type ca struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	path string
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()

	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newCA(t *testing.T, dir string) ca {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "ca-"+big.NewInt(time.Now().UnixNano()).String()+".pem")
	writePEM(t, path, "CERTIFICATE", der)

	return ca{cert: cert, key: key, path: path}
}

// issue writes a certificate for a service identity and returns its config.
func (c ca) issue(t *testing.T, dir, name string) mtls.Config {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{"localhost", name},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		t.Fatal(err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	certPath := filepath.Join(dir, name+".pem")
	keyPath := filepath.Join(dir, name+"-key.pem")
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "EC PRIVATE KEY", keyDER)

	return mtls.Config{CAPath: c.path, CertPath: certPath, KeyPath: keyPath}
}

const restricted = "/grpc.health.v1.Health/Check"

// serve starts a health server that lets only "auth" call Check and
// reports which identity it saw.
func serve(t *testing.T, cfg mtls.Config) (addr string, seen <-chan string) {
	t.Helper()

	creds, err := mtls.ServerCredentials(cfg)
	if err != nil {
		t.Fatalf("ServerCredentials: %v", err)
	}

	identities := make(chan string, 10)
	record := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		id, _ := mtls.PeerIdentity(ctx)
		identities <- id
		return h(ctx, req)
	}

	srv := grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(record, mtls.RestrictMethods(map[string][]string{restricted: {mtls.Auth}})),
	)
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return lis.Addr().String(), identities
}

func check(t *testing.T, addr string, creds grpc.DialOption) error {
	t.Helper()

	conn, err := grpc.NewClient(addr, creds)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})

	return err
}

func clientCreds(t *testing.T, cfg mtls.Config) grpc.DialOption {
	t.Helper()

	creds, err := mtls.ClientCredentials(cfg)
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}

	return grpc.WithTransportCredentials(creds)
}

func TestMutualTLS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	internal := newCA(t, dir)
	addr, seen := serve(t, internal.issue(t, dir, mtls.User))

	t.Run("allowed service passes and is identified", func(t *testing.T) {
		if err := check(t, addr, clientCreds(t, internal.issue(t, dir, mtls.Auth))); err != nil {
			t.Fatalf("auth: %v", err)
		}
		if id := <-seen; id != mtls.Auth {
			t.Fatalf("identity = %q, want %q", id, mtls.Auth)
		}
	})

	t.Run("other service is denied the restricted method", func(t *testing.T) {
		err := check(t, addr, clientCreds(t, internal.issue(t, dir, mtls.Gateway)))
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("gateway: got %v, want PermissionDenied", err)
		}
	})

	t.Run("plaintext client cannot connect", func(t *testing.T) {
		err := check(t, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("got %v, want Unavailable", err)
		}
	})

	t.Run("certificate from a foreign CA is rejected", func(t *testing.T) {
		foreignDir := t.TempDir()
		foreign := newCA(t, foreignDir).issue(t, foreignDir, mtls.Auth)
		// Trust our server, but present a certificate signed elsewhere.
		foreign.CAPath = internal.path

		if err := check(t, addr, clientCreds(t, foreign)); status.Code(err) != codes.Unavailable {
			t.Fatalf("got %v, want Unavailable", err)
		}
	})
}

// replace puts the certificate and the key of from in the place of
// those of to, as a renewal does, with a time of their own.
func replace(t *testing.T, from, to mtls.Config, files ...string) {
	t.Helper()

	later := time.Now().Add(time.Hour)

	for _, pair := range [][2]string{{from.CertPath, to.CertPath}, {from.KeyPath, to.KeyPath}} {
		if len(files) > 0 && pair[1] != files[0] {
			continue
		}
		if err := os.Chtimes(pair[0], later, later); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCredentials_RenewedCertificate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	internal := newCA(t, dir)
	addr, seen := serve(t, internal.issue(t, dir, mtls.User))

	cfg := internal.issue(t, dir, mtls.Auth)
	creds := clientCreds(t, cfg)

	if err := check(t, addr, creds); err != nil {
		t.Fatalf("before: %v", err)
	}
	if id := <-seen; id != mtls.Auth {
		t.Fatalf("identity = %q, want %q", id, mtls.Auth)
	}

	// Only the certificate so far: it does not match the key, the one
	// loaded is still presented.
	renewed := internal.issue(t, t.TempDir(), mtls.Post)
	replace(t, renewed, cfg, cfg.CertPath)

	if err := check(t, addr, creds); status.Code(err) != codes.OK {
		t.Fatalf("half replaced: %v", err)
	}
	if id := <-seen; id != mtls.Auth {
		t.Fatalf("half replaced: identity = %q, want %q", id, mtls.Auth)
	}

	replace(t, renewed, cfg, cfg.KeyPath)

	// The same credentials, no restart: the next connection presents the
	// new certificate.
	if err := check(t, addr, creds); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("after: got %v, want PermissionDenied for %s", err, mtls.Post)
	}
	if id := <-seen; id != mtls.Post {
		t.Fatalf("identity = %q, want %q", id, mtls.Post)
	}
}

func TestServerCredentials_RenewedCertificate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	internal := newCA(t, dir)
	cfg := internal.issue(t, dir, mtls.User)
	addr, _ := serve(t, cfg)
	creds := clientCreds(t, internal.issue(t, dir, mtls.Auth))

	if err := check(t, addr, creds); err != nil {
		t.Fatalf("before: %v", err)
	}

	// A certificate of another CA in its place: the client, which trusts
	// only ours, turns the server down from now on.
	foreignDir := t.TempDir()
	replace(t, newCA(t, foreignDir).issue(t, foreignDir, mtls.User), cfg)

	if err := check(t, addr, creds); status.Code(err) != codes.Unavailable {
		t.Fatalf("after: got %v, want Unavailable", err)
	}
}

func TestRestrictMethods_WithoutTLS(t *testing.T) {
	t.Parallel()

	addr, _ := serve(t, mtls.Config{})

	err := check(t, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("no certificate means no identity: got %v, want PermissionDenied", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	if err := (mtls.Config{}).Validate(); err != nil {
		t.Fatalf("disabled: %v", err)
	}
	if err := (mtls.Config{CAPath: "a", CertPath: "b", KeyPath: "c"}).Validate(); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := (mtls.Config{CAPath: "a"}).Validate(); err == nil {
		t.Fatal("partial config must fail")
	}
}
