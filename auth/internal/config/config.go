package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/mtls"
)

const (
	// StorageInmem keeps sessions, codes and limits in process memory:
	// nothing to run locally, everything is lost on restart.
	StorageInmem = "inmem"
	// StoragePersistent keeps sessions in PostgreSQL and codes and limits
	// in Redis.
	StoragePersistent = "persistent"

	// MailerConsole writes codes to the log; development only.
	MailerConsole = "console"
	MailerSMTP    = "smtp"
)

type Config struct {
	GRPCPort  int    `env:"GRPC_PORT,required,notEmpty"`
	LogLevel  string `env:"LOG_LEVEL,required,notEmpty"`
	LogFormat string `env:"LOG_FORMAT,required,notEmpty"`

	// MetricsPort serves Prometheus metrics on a port of its own, not to
	// be published outside; 0 does not serve them.
	MetricsPort int `env:"METRICS_PORT"`

	// mTLS with other services, as server and as client of user:
	// TLS_CA_PATH, TLS_CERT_PATH, TLS_KEY_PATH.
	TLS mtls.Config `envPrefix:"TLS_"`

	UserServiceAddr string `env:"USER_SERVICE_ADDR,required,notEmpty"`

	Storage     string `env:"STORAGE" envDefault:"inmem"`
	DatabaseURL string `env:"DATABASE_URL"`
	RedisAddr   string `env:"REDIS_ADDR"`
	// MigrateOnStart applies pending migrations before serving.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"true"`

	Mailer string     `env:"MAILER" envDefault:"console"`
	SMTP   SMTPConfig `envPrefix:"SMTP_"`

	HashSecret        string `env:"HASH_SECRET,required,notEmpty"`
	JWTPrivateKeyPath string `env:"JWT_PRIVATE_KEY_PATH,required,notEmpty"`
	JWTPublicKeyPath  string `env:"JWT_PUBLIC_KEY_PATH,required,notEmpty"`
	Issuer            string `env:"ISSUER,required,notEmpty"`

	AccessTokenTTL  time.Duration `env:"ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"720h"`

	Code CodeConfig `envPrefix:"CODE_"`
}

type SMTPConfig struct {
	Host     string `env:"HOST"`
	Port     string `env:"PORT"`
	Username string `env:"USERNAME"`
	Password string `env:"PASSWORD"`
	From     string `env:"FROM"`
}

type CodeConfig struct {
	TTL            time.Duration `env:"TTL" envDefault:"5m"`
	MaxAttempts    int           `env:"MAX_ATTEMPTS" envDefault:"5"`
	ResendInterval time.Duration `env:"RESEND_INTERVAL" envDefault:"1m"`
	RequestsPerIP  int           `env:"REQUESTS_PER_IP" envDefault:"10"`
	RequestsWindow time.Duration `env:"REQUESTS_WINDOW" envDefault:"1h"`
}

func Load() (Config, error) {
	return envconfig.Load[Config]()
}

func (c Config) Validate() error {
	switch c.Storage {
	case StorageInmem:
	case StoragePersistent:
		if c.DatabaseURL == "" || c.RedisAddr == "" {
			return errors.New("STORAGE=persistent requires DATABASE_URL and REDIS_ADDR")
		}
	default:
		return fmt.Errorf("unknown STORAGE %q, want %q or %q", c.Storage, StorageInmem, StoragePersistent)
	}

	switch c.Mailer {
	case MailerConsole:
	case MailerSMTP:
		if c.SMTP.Host == "" || c.SMTP.Port == "" || c.SMTP.From == "" {
			return errors.New("MAILER=smtp requires SMTP_HOST, SMTP_PORT and SMTP_FROM")
		}
	default:
		return fmt.Errorf("unknown MAILER %q, want %q or %q", c.Mailer, MailerConsole, MailerSMTP)
	}

	if c.Code.TTL <= 0 || c.Code.ResendInterval <= 0 || c.Code.RequestsWindow <= 0 ||
		c.Code.MaxAttempts <= 0 || c.Code.RequestsPerIP <= 0 {
		return errors.New("CODE_* settings must be positive")
	}

	return c.TLS.Validate()
}

func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.Wrap("os.ReadFile", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errs.Wrap("x509.ParsePKCS1PrivateKey", err)
	}

	return privateKey, nil
}
