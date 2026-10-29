// Package mtls secures traffic between freenet services: every service
// presents a certificate from the internal CA and accepts only peers that
// present one too. The certificate's common name is the service identity.
package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Service identities: the common names in the certificates from `make certs`.
const (
	Auth         = "auth"
	User         = "user"
	Post         = "post"
	Media        = "media"
	Message      = "message"
	Notification = "notification"
	Club         = "club"
	Gateway      = "gateway"
)

// Config is embedded by services with envPrefix "TLS_": TLS_CA_PATH,
// TLS_CERT_PATH, TLS_KEY_PATH. All three or none; none means plaintext,
// which is for local development only.
type Config struct {
	CAPath   string `env:"CA_PATH"`
	CertPath string `env:"CERT_PATH"`
	KeyPath  string `env:"KEY_PATH"`
}

func (c Config) Enabled() bool {
	return c.CAPath != ""
}

func (c Config) Validate() error {
	set := 0
	for _, v := range []string{c.CAPath, c.CertPath, c.KeyPath} {
		if v != "" {
			set++
		}
	}

	if set != 0 && set != 3 {
		return errors.New("TLS_CA_PATH, TLS_CERT_PATH and TLS_KEY_PATH must be set together")
	}

	return nil
}

// ServerCredentials require a client certificate signed by the CA. The
// certificate is read again when its files change, see keeper.
func ServerCredentials(c Config) (credentials.TransportCredentials, error) {
	if !c.Enabled() {
		return insecure.NewCredentials(), nil
	}

	own, pool, err := load(c)
	if err != nil {
		return nil, err
	}

	return credentials.NewTLS(&tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return own.current(), nil
		},
		ClientCAs:  pool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}), nil
}

// ClientCredentials present our certificate and trust only servers signed
// by the CA.
func ClientCredentials(c Config) (credentials.TransportCredentials, error) {
	if !c.Enabled() {
		return insecure.NewCredentials(), nil
	}

	own, pool, err := load(c)
	if err != nil {
		return nil, err
	}

	return credentials.NewTLS(&tls.Config{
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return own.current(), nil
		},
		RootCAs:    pool,
		MinVersion: tls.VersionTLS13,
	}), nil
}

func load(c Config) (*keeper, *x509.CertPool, error) {
	own, err := newKeeper(c.CertPath, c.KeyPath)
	if err != nil {
		return nil, nil, err
	}

	caPEM, err := os.ReadFile(c.CAPath)
	if err != nil {
		return nil, nil, fmt.Errorf("os.ReadFile: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, nil, fmt.Errorf("no certificates in %s", c.CAPath)
	}

	return own, pool, nil
}

// keeper holds the certificate of the service and reads it again once its
// files have changed, on the next handshake: a certificate renewed on disk
// (deploy/certs.sh) is presented without a restart. Connections already
// open keep the one they were made with.
type keeper struct {
	certPath string
	keyPath  string

	mu    sync.Mutex
	cert  *tls.Certificate
	stamp stamp
}

// stamp tells one pair of files from the next.
type stamp struct {
	cert time.Time
	key  time.Time
}

func newKeeper(certPath, keyPath string) (*keeper, error) {
	k := &keeper{certPath: certPath, keyPath: keyPath}

	seen, err := k.seen()
	if err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("tls.LoadX509KeyPair: %w", err)
	}

	k.cert, k.stamp = &cert, seen

	return k, nil
}

func (k *keeper) seen() (stamp, error) {
	cert, err := os.Stat(k.certPath)
	if err != nil {
		return stamp{}, fmt.Errorf("os.Stat: %w", err)
	}

	key, err := os.Stat(k.keyPath)
	if err != nil {
		return stamp{}, fmt.Errorf("os.Stat: %w", err)
	}

	return stamp{cert: cert.ModTime(), key: key.ModTime()}, nil
}

// current is the certificate on disk, or the last one that could be read:
// files that are missing or do not match yet, a pair half replaced, are
// looked at again on the next handshake.
func (k *keeper) current() *tls.Certificate {
	k.mu.Lock()
	defer k.mu.Unlock()

	seen, err := k.seen()
	if err != nil || seen == k.stamp {
		return k.cert
	}

	cert, err := tls.LoadX509KeyPair(k.certPath, k.keyPath)
	if err != nil {
		return k.cert
	}

	k.cert, k.stamp = &cert, seen

	return k.cert
}
