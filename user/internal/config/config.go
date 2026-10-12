package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/user/internal/user"
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

	// MetricsPort serves Prometheus metrics on a port of its own, not to
	// be published outside; 0 does not serve them.
	MetricsPort int `env:"METRICS_PORT"`

	// mTLS with other services: TLS_CA_PATH, TLS_CERT_PATH, TLS_KEY_PATH.
	TLS mtls.Config `envPrefix:"TLS_"`

	Storage     string `env:"STORAGE" envDefault:"inmem"`
	DatabaseURL string `env:"DATABASE_URL"`
	// MigrateOnStart applies pending migrations before serving.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"true"`

	// NotificationServiceAddr is optional: without it follows notify nobody.
	NotificationServiceAddr string `env:"NOTIFICATION_SERVICE_ADDR"`
	// The services that keep data of users, told to erase a page deleted
	// for good; one left unset keeps the data of erased users.
	PostServiceAddr    string `env:"POST_SERVICE_ADDR"`
	MediaServiceAddr   string `env:"MEDIA_SERVICE_ADDR"`
	MessageServiceAddr string `env:"MESSAGE_SERVICE_ADDR"`
	ClubServiceAddr    string `env:"CLUB_SERVICE_ADDR"`
	// NotifyTimeout bounds one delivery to another service.
	NotifyTimeout time.Duration `env:"NOTIFY_TIMEOUT" envDefault:"2s"`

	// Spam protection, per user. REDIS_ADDR shares the counters between
	// replicas; without it they are kept in memory, per replica.
	RedisAddr    string          `env:"REDIS_ADDR"`
	FollowLimits ratelimit.Rules `env:"RATE_LIMIT_FOLLOWS" envDefault:"30/1m,300/24h"`
	GiftLimits   ratelimit.Rules `env:"RATE_LIMIT_GIFTS" envDefault:"10/1m,100/24h"`

	// AccountRetention is how long a deleted page can be restored before
	// its data is erased; openvk keeps it seven months.
	AccountRetention time.Duration `env:"ACCOUNT_RETENTION" envDefault:"5040h"`
	// AccountPurgeInterval is how often the pages that are due are erased.
	AccountPurgeInterval time.Duration `env:"ACCOUNT_PURGE_INTERVAL" envDefault:"1m"`

	// PresenceFlushInterval is how often the visits of the people on the
	// site are written, all at once: a person shows as online that much
	// later.
	PresenceFlushInterval time.Duration `env:"PRESENCE_FLUSH_INTERVAL" envDefault:"10s"`

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

	if c.AccountRetention <= 0 || c.AccountPurgeInterval <= 0 {
		return errors.New("ACCOUNT_RETENTION and ACCOUNT_PURGE_INTERVAL must be positive")
	}

	if c.PresenceFlushInterval <= 0 || c.PresenceFlushInterval >= user.OnlineFor {
		return fmt.Errorf("PRESENCE_FLUSH_INTERVAL must be positive and below %s, how long a visit counts as online", user.OnlineFor)
	}

	return c.TLS.Validate()
}
