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
	// StorageInmem keeps media in process memory: nothing to set up,
	// everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps media as files under MEDIA_DIR.
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

	Storage  string `env:"STORAGE" envDefault:"inmem"`
	MediaDir string `env:"MEDIA_DIR"`

	// A photo uploaded for a post that is never published is deleted
	// after UnpublishedPhotoTTL; the uploads are looked through every
	// PhotoCleanupInterval.
	UnpublishedPhotoTTL  time.Duration `env:"UNPUBLISHED_PHOTO_TTL" envDefault:"24h"`
	PhotoCleanupInterval time.Duration `env:"PHOTO_CLEANUP_INTERVAL" envDefault:"1h"`

	// Spam protection, per user. REDIS_ADDR shares the counters between
	// replicas; without it they are kept in memory, per replica.
	RedisAddr    string          `env:"REDIS_ADDR"`
	PhotoLimits  ratelimit.Rules `env:"RATE_LIMIT_PHOTOS" envDefault:"30/1m,300/24h"`
	AvatarLimits ratelimit.Rules `env:"RATE_LIMIT_AVATARS" envDefault:"5/1m,30/24h"`

	// Access tokens are issued by the auth service; media only verifies them.
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
		if c.MediaDir == "" {
			return errors.New("STORAGE=persistent requires MEDIA_DIR")
		}
	default:
		return fmt.Errorf("unknown STORAGE %q, want %q or %q", c.Storage, StorageInmem, StoragePersistent)
	}

	if c.UnpublishedPhotoTTL <= 0 || c.PhotoCleanupInterval <= 0 {
		return errors.New("UNPUBLISHED_PHOTO_TTL and PHOTO_CLEANUP_INTERVAL must be positive")
	}

	return c.TLS.Validate()
}
