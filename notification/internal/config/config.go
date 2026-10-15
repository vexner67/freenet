package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/tracing"
)

const (
	// StorageInmem keeps notifications in process memory: nothing to run locally,
	// everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps notifications in PostgreSQL.
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

	// RedisAddr shares live events between replicas; without it a stream
	// gets only the events of its replica.
	RedisAddr string `env:"REDIS_ADDR"`

	// Retention is how long a notification is kept: older ones are
	// deleted, read or not. CleanupInterval is how often.
	Retention       time.Duration `env:"NOTIFICATION_RETENTION" envDefault:"4320h"`
	CleanupInterval time.Duration `env:"NOTIFICATION_CLEANUP_INTERVAL" envDefault:"1h"`

	// PushKeyPath is the VAPID private key (P-256, PEM) pushes to browsers
	// are signed with; empty keeps pushes off. PushSubject is whom the push
	// services may contact: a mailto: or https URL.
	PushKeyPath string `env:"PUSH_KEY_PATH"`
	PushSubject string `env:"PUSH_SUBJECT" envDefault:"https://github.com/vexner67/freenet"`
	// PushTimeout bounds pushing one event to all browsers of a user;
	// PushQueue is how many may wait (more are dropped), PushWorkers how
	// many are pushed at once.
	PushTimeout time.Duration `env:"PUSH_TIMEOUT" envDefault:"10s"`
	PushQueue   int           `env:"PUSH_QUEUE" envDefault:"1000"`
	PushWorkers int           `env:"PUSH_WORKERS" envDefault:"4"`

	// UserServiceAddr and ClubServiceAddr name who did what a push tells
	// of, within PEOPLE_TIMEOUT; without them pushes name nobody.
	UserServiceAddr string        `env:"USER_SERVICE_ADDR"`
	ClubServiceAddr string        `env:"CLUB_SERVICE_ADDR"`
	PeopleTimeout   time.Duration `env:"PEOPLE_TIMEOUT" envDefault:"2s"`

	// Access tokens are issued by the auth service; notification only verifies them.
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

	if c.Retention <= 0 || c.CleanupInterval <= 0 {
		return errors.New("NOTIFICATION_RETENTION and NOTIFICATION_CLEANUP_INTERVAL must be positive")
	}

	if !strings.HasPrefix(c.PushSubject, "mailto:") && !strings.HasPrefix(c.PushSubject, "https://") {
		return errors.New("PUSH_SUBJECT must be a mailto: or https URL")
	}

	if c.PushTimeout <= 0 || c.PushQueue <= 0 || c.PushWorkers <= 0 || c.PeopleTimeout <= 0 {
		return errors.New("PUSH_TIMEOUT, PUSH_QUEUE, PUSH_WORKERS and PEOPLE_TIMEOUT must be positive")
	}

	if err := c.Tracing.Validate(); err != nil {
		return err
	}

	return c.TLS.Validate()
}
