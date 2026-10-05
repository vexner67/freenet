package config

import (
	"errors"
	"fmt"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
)

const (
	// StorageInmem keeps users in process memory: nothing to run locally,
	// everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps users in PostgreSQL.
	StoragePersistent = "persistent"
)

type Config struct {
	GRPCPort  int    `env:"GRPC_PORT,required,notEmpty"`
	LogLevel  string `env:"LOG_LEVEL,required,notEmpty"`
	LogFormat string `env:"LOG_FORMAT,required,notEmpty"`

	// mTLS with other services: TLS_CA_PATH, TLS_CERT_PATH, TLS_KEY_PATH.
	TLS mtls.Config `envPrefix:"TLS_"`

	Storage     string `env:"STORAGE" envDefault:"inmem"`
	DatabaseURL string `env:"DATABASE_URL"`
	// MigrateOnStart applies pending migrations before serving.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"true"`

	// Access tokens are issued by the auth service; user only verifies them.
	JWTPublicKeyPath string `env:"JWT_PUBLIC_KEY_PATH,required,notEmpty"`
	JWTIssuer        string `env:"JWT_ISSUER,required,notEmpty"`
}

func Load() (Config, error) {
	return envconfig.Load[Config]()
}

func (c Config) Validate() error {
	switch c.Storage {
	case StorageInmem:
	case StoragePersistent:
		if c.DatabaseURL == "" {
			return errors.New("STORAGE=persistent requires DATABASE_URL")
		}
	default:
		return fmt.Errorf("unknown STORAGE %q, want %q or %q", c.Storage, StorageInmem, StoragePersistent)
	}

	return c.TLS.Validate()
}
