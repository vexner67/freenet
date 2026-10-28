// Package config is the configuration of the world service, read from the
// environment.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/tracing"
)

const (
	// StorageInmem keeps the worlds in process memory: nothing to run
	// locally, everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps the worlds in PostgreSQL.
	StoragePersistent = "persistent"
)

type Config struct {
	GRPCPort  int    `env:"GRPC_PORT,required,notEmpty"`
	LogLevel  string `env:"LOG_LEVEL,required,notEmpty"`
	LogFormat string `env:"LOG_FORMAT,required,notEmpty"`

	// MetricsPort serves Prometheus metrics on a port of its own, not to
	// be published outside; 0 does not serve them.
	MetricsPort int `env:"METRICS_PORT"`

	// mTLS with other services: TLS_CA_PATH, TLS_CERT_PATH, TLS_KEY_PATH.
	TLS mtls.Config `envPrefix:"TLS_"`

	// Tracing of requests through the services: TRACING_ENDPOINT, the
	// collector (without it nothing is traced), TRACING_SAMPLE_RATIO.
	Tracing tracing.Config `envPrefix:"TRACING_"`

	Storage     string `env:"STORAGE" envDefault:"inmem"`
	DatabaseURL string `env:"DATABASE_URL"`
	// MigrateOnStart applies pending migrations before serving.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"true"`

	// UserServiceAddr takes and pays the votes of states.
	UserServiceAddr string `env:"USER_SERVICE_ADDR,required,notEmpty"`
	// ClubServiceAddr is optional: without it the states have no groups.
	ClubServiceAddr string `env:"CLUB_SERVICE_ADDR"`
	// CallTimeout bounds one call to another service.
	CallTimeout time.Duration `env:"CALL_TIMEOUT" envDefault:"3s"`

	// WorldInterval is how often the hours of the world are counted and
	// the groups of new states made.
	WorldInterval time.Duration `env:"WORLD_INTERVAL" envDefault:"1m"`
	// GenerateInterval is how often a replica looks for a world whose map
	// is to be made, besides being told by CreateWorld.
	GenerateInterval time.Duration `env:"GENERATE_INTERVAL" envDefault:"1m"`
	// TileWidth is the width of the map at the closest zoom, in pixels:
	// 512 times a power of two.
	TileWidth int `env:"TILE_WIDTH" envDefault:"4096"`

	// Spam protection, per user. REDIS_ADDR shares the counters between
	// replicas; without it each counts on its own.
	RedisAddr    string          `env:"REDIS_ADDR"`
	ActionLimits ratelimit.Rules `env:"RATE_LIMIT_WORLD_ACTIONS" envDefault:"30/1m,300/24h"`

	// Access tokens are issued by the auth service; world only verifies
	// them.
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

	if c.WorldInterval <= 0 || c.GenerateInterval <= 0 {
		return errors.New("WORLD_INTERVAL and GENERATE_INTERVAL must be positive")
	}

	if err := c.Tracing.Validate(); err != nil {
		return err
	}

	return c.TLS.Validate()
}
