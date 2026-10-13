// Package config is the configuration of the club service, read from the
// environment.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/ratelimit"
)

const (
	// StorageInmem keeps clubs in process memory: nothing to run locally,
	// everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps clubs in PostgreSQL.
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

	Storage     string `env:"STORAGE" envDefault:"inmem"`
	DatabaseURL string `env:"DATABASE_URL"`
	// MigrateOnStart applies pending migrations before serving.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"true"`

	// NotificationServiceAddr is optional: without it an appointment
	// notifies nobody. NotifyTimeout bounds one delivery.
	NotificationServiceAddr string        `env:"NOTIFICATION_SERVICE_ADDR"`
	NotifyTimeout           time.Duration `env:"NOTIFY_TIMEOUT" envDefault:"2s"`

	// PostServiceAddr is optional: without it the news of post do not
	// show the walls of the clubs their readers are in.
	PostServiceAddr string `env:"POST_SERVICE_ADDR"`

	// FeedSyncInterval is how often post is told anew who is in every
	// club, which mends its copy after a word lost on the way; 0: never.
	FeedSyncInterval time.Duration `env:"FEED_SYNC_INTERVAL" envDefault:"6h"`

	// EventReminderAhead is how long before an event starts its members
	// are reminded of it; EventReminderInterval how often the events that
	// are due are looked for.
	EventReminderAhead    time.Duration `env:"EVENT_REMINDER_AHEAD" envDefault:"24h"`
	EventReminderInterval time.Duration `env:"EVENT_REMINDER_INTERVAL" envDefault:"5m"`

	// Spam protection, per user. REDIS_ADDR shares the counters between
	// replicas; without it each counts on its own.
	RedisAddr  string          `env:"REDIS_ADDR"`
	ClubLimits ratelimit.Rules `env:"RATE_LIMIT_CLUBS" envDefault:"3/1h,10/24h"`
	EditLimits ratelimit.Rules `env:"RATE_LIMIT_CLUB_EDITS" envDefault:"10/1m,100/24h"`
	JoinLimits ratelimit.Rules `env:"RATE_LIMIT_JOINS" envDefault:"30/1m,300/24h"`

	// Access tokens are issued by the auth service; club only verifies them.
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

	if c.FeedSyncInterval < 0 {
		return errors.New("FEED_SYNC_INTERVAL must not be negative")
	}

	if c.EventReminderAhead <= 0 || c.EventReminderInterval <= 0 {
		return errors.New("EVENT_REMINDER_AHEAD and EVENT_REMINDER_INTERVAL must be positive")
	}

	return c.TLS.Validate()
}
