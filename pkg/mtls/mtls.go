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

// ServerCredentials require a client certificate signed by the CA.
func ServerCredentials(c Config) (credentials.TransportCredentials, error) {
	if !c.Enabled() {
		return insecure.NewCredentials(), nil
	}

	cert, pool, err := load(c)
	if err != nil {
		return nil, err
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

// ClientCredentials present our certificate and trust only servers signed
// by the CA.
func ClientCredentials(c Config) (credentials.TransportCredentials, error) {
	if !c.Enabled() {
		return insecure.NewCredentials(), nil
	}

	cert, pool, err := load(c)
	if err != nil {
		return nil, err
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

func load(c Config) (tls.Certificate, *x509.CertPool, error) {
	cert, err := tls.LoadX509KeyPair(c.CertPath, c.KeyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("tls.LoadX509KeyPair: %w", err)
	}

	caPEM, err := os.ReadFile(c.CAPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("os.ReadFile: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return tls.Certificate{}, nil, fmt.Errorf("no certificates in %s", c.CAPath)
	}

	return cert, pool, nil
}
