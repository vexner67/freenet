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

	// NotificationServiceAddr is optional: without it likes and comments
	// notify nobody.
	NotificationServiceAddr string        `env:"NOTIFICATION_SERVICE_ADDR"`
	NotifyTimeout           time.Duration `env:"NOTIFY_TIMEOUT" envDefault:"2s"`

	// MediaServiceAddr is where post tells which photos its posts publish
	// and delete, within NOTIFY_TIMEOUT too. Without it media takes the
	// photos of posts for never published and deletes them.
	MediaServiceAddr string `env:"MEDIA_SERVICE_ADDR"`

	// ClubServiceAddr is whom post asks, within NOTIFY_TIMEOUT too, whether
	// someone keeps a community's wall clean. Without it only the authors
	// delete their posts and comments on community walls.
	ClubServiceAddr string `env:"CLUB_SERVICE_ADDR"`

	// Spam protection, per user. REDIS_ADDR shares the counters between
	// replicas; without it they are kept in memory, per replica. Posts and
	// reposts both follow RATE_LIMIT_POSTS, each counted on its own.
	RedisAddr     string          `env:"REDIS_ADDR"`
	PostLimits    ratelimit.Rules `env:"RATE_LIMIT_POSTS" envDefault:"5/1m,100/24h"`
	CommentLimits ratelimit.Rules `env:"RATE_LIMIT_COMMENTS" envDefault:"20/1m,500/24h"`
	LikeLimits    ratelimit.Rules `env:"RATE_LIMIT_LIKES" envDefault:"60/1m,1000/24h"`
	EditLimits    ratelimit.Rules `env:"RATE_LIMIT_POST_EDITS" envDefault:"10/1m,200/24h"`

	// PostponedInterval is how often the postponed posts whose time has
	// come are published: a post comes out up to this late.
	PostponedInterval time.Duration `env:"POSTPONED_INTERVAL" envDefault:"30s"`
	// StoryCleanupInterval is how often the stories whose day is over are
	// deleted with their photos; until then the lists leave them out.
	StoryCleanupInterval time.Duration `env:"STORY_CLEANUP_INTERVAL" envDefault:"10m"`

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

	if err := c.Tracing.Validate(); err != nil {
		return err
	}

	if c.PostponedInterval <= 0 {
		return errors.New("POSTPONED_INTERVAL must be positive")
	}

	if c.StoryCleanupInterval <= 0 {
		return errors.New("STORY_CLEANUP_INTERVAL must be positive")
	}

	return c.TLS.Validate()
}
