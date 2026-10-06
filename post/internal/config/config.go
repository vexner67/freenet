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
	// StorageInmem keeps posts in process memory: nothing to run locally,
	// everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps posts in PostgreSQL.
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

	// NotificationServiceAddr is optional: without it likes and comments
	// notify nobody.
	NotificationServiceAddr string        `env:"NOTIFICATION_SERVICE_ADDR"`
	NotifyTimeout           time.Duration `env:"NOTIFY_TIMEOUT" envDefault:"2s"`

	// Spam protection, per user. REDIS_ADDR shares the counters between
	// replicas; without it they are kept in memory, per replica.
	RedisAddr     string          `env:"REDIS_ADDR"`
	PostLimits    ratelimit.Rules `env:"RATE_LIMIT_POSTS" envDefault:"5/1m,100/24h"`
	CommentLimits ratelimit.Rules `env:"RATE_LIMIT_COMMENTS" envDefault:"20/1m,500/24h"`
	LikeLimits    ratelimit.Rules `env:"RATE_LIMIT_LIKES" envDefault:"60/1m,1000/24h"`

	// Access tokens are issued by the auth service; post only verifies them.
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
