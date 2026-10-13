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
	// StoragePersistent keeps media as files under MEDIA_DIR: one node.
	StoragePersistent = "persistent"
	// StorageS3 keeps media in an S3 bucket, shared by all replicas.
	StorageS3 = "s3"
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

	// STORAGE=s3: the bucket, which must exist, at an S3-compatible
	// storage that takes conditional writes (If-None-Match), as AWS S3
	// and MinIO do. S3_PREFIX goes before every key; S3_INSECURE talks
	// plain HTTP, as to a MinIO next to the service.
	S3Endpoint  string `env:"S3_ENDPOINT"`
	S3Region    string `env:"S3_REGION" envDefault:"us-east-1"`
	S3Bucket    string `env:"S3_BUCKET"`
	S3AccessKey string `env:"S3_ACCESS_KEY"`
	S3SecretKey string `env:"S3_SECRET_KEY"`
	S3Prefix    string `env:"S3_PREFIX"`
	S3Insecure  bool   `env:"S3_INSECURE"`

	// A photo uploaded for a post that is never published is deleted
	// after UnpublishedPhotoTTL; the uploads are looked through every
	// PhotoCleanupInterval.
	UnpublishedPhotoTTL time.Duration `env:"UNPUBLISHED_PHOTO_TTL" envDefault:"24h"`
	// It is set aside, not gone, until RetiredPhotoTTL after its upload:
	// a post published while the outbox of its service was stuck still
	// gets its photo when the word arrives.
	RetiredPhotoTTL      time.Duration `env:"RETIRED_PHOTO_TTL" envDefault:"720h"`
	PhotoCleanupInterval time.Duration `env:"PHOTO_CLEANUP_INTERVAL" envDefault:"1h"`
	// A photo claimed for a post that failed without a word on whether
	// it was written stays claimed; once the claim is PhotoClaimTTL old
	// and nothing has published the photo, its owner may post it again.
	// 0: never, the claim holds until the upload is taken away.
	PhotoClaimTTL time.Duration `env:"PHOTO_CLAIM_TTL" envDefault:"15m"`

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
	case StorageS3:
		if c.S3Endpoint == "" || c.S3Bucket == "" || c.S3AccessKey == "" || c.S3SecretKey == "" {
			return errors.New("STORAGE=s3 requires S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY and S3_SECRET_KEY")
		}
	default:
		return fmt.Errorf("unknown STORAGE %q, want %q, %q or %q", c.Storage, StorageInmem, StoragePersistent, StorageS3)
	}

	if c.UnpublishedPhotoTTL <= 0 || c.PhotoCleanupInterval <= 0 {
		return errors.New("UNPUBLISHED_PHOTO_TTL and PHOTO_CLEANUP_INTERVAL must be positive")
	}

	if c.PhotoClaimTTL < 0 {
		return errors.New("PHOTO_CLAIM_TTL must not be negative")
	}

	if c.RetiredPhotoTTL < c.UnpublishedPhotoTTL {
		return errors.New("RETIRED_PHOTO_TTL must not be shorter than UNPUBLISHED_PHOTO_TTL")
	}

	return c.TLS.Validate()
}
