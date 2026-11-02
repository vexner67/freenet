package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/tracing"
)

const (
	// StorageInmem keeps federation in process memory: nothing to run
	// locally, everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps it in PostgreSQL.
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

	// PublicURL is where the site is on the web, which the addresses of
	// its pages in the Fediverse begin with: https://<domain>.
	PublicURL string `env:"PUBLIC_URL" envDefault:"http://localhost:8080"`

	// The pages and posts are read from the user and post services, each
	// call within SERVICES_TIMEOUT.
	UserServiceAddr string        `env:"USER_SERVICE_ADDR,required,notEmpty"`
	PostServiceAddr string        `env:"POST_SERVICE_ADDR,required,notEmpty"`
	ServicesTimeout time.Duration `env:"SERVICES_TIMEOUT" envDefault:"2s"`

	// RemoteTimeout bounds a request to another server; an activity it
	// does not take is sent again until DELIVERY_MAX_AGE after it was made.
	RemoteTimeout  time.Duration `env:"REMOTE_TIMEOUT" envDefault:"10s"`
	DeliveryMaxAge time.Duration `env:"DELIVERY_MAX_AGE" envDefault:"48h"`

	// FeedLanguages are the languages of the remote posts in the news of
	// everyone; a post that does not say its language is there too.
	FeedLanguages []string `env:"FEED_LANGUAGES" envDefault:"ru,en"`
	// NewsRetention is how long remote posts are kept; CleanupInterval,
	// how often older ones are forgotten.
	NewsRetention   time.Duration `env:"NEWS_RETENTION" envDefault:"2160h"`
	CleanupInterval time.Duration `env:"CLEANUP_INTERVAL" envDefault:"1h"`

	// Access tokens are issued by the auth service; federation only
	// verifies them.
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

	if _, err := fedi.NewSite(c.PublicURL); err != nil {
		return fmt.Errorf("PUBLIC_URL %q is not the address of a site, e.g. https://example.com", c.PublicURL)
	}

	if c.ServicesTimeout <= 0 || c.RemoteTimeout <= 0 || c.DeliveryMaxAge <= 0 || c.NewsRetention <= 0 || c.CleanupInterval <= 0 {
		return errors.New("SERVICES_TIMEOUT, REMOTE_TIMEOUT, DELIVERY_MAX_AGE, NEWS_RETENTION and CLEANUP_INTERVAL must be positive")
	}

	if err := c.Tracing.Validate(); err != nil {
		return err
	}

	return c.TLS.Validate()
}
