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
	"github.com/vexner67/freenet/pkg/tracing"
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
	// MailerTelegram sends codes to a Telegram chat of the site's owner,
	// who passes them on: while the users are the owner's friends.
	MailerTelegram = "telegram"
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

	// Tracing of requests through the services: TRACING_ENDPOINT, the
	// collector (without it nothing is traced), TRACING_SAMPLE_RATIO.
	Tracing tracing.Config `envPrefix:"TRACING_"`

	UserServiceAddr string `env:"USER_SERVICE_ADDR,required,notEmpty"`

	Storage     string `env:"STORAGE" envDefault:"inmem"`
	DatabaseURL string `env:"DATABASE_URL"`
	RedisAddr   string `env:"REDIS_ADDR"`
	// MigrateOnStart applies pending migrations before serving.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"true"`

	Mailer string     `env:"MAILER" envDefault:"console"`
	SMTP   SMTPConfig `envPrefix:"SMTP_"`
	// Telegram is the bot and the chat of MAILER=telegram.
	Telegram TelegramConfig `envPrefix:"TELEGRAM_"`

	HashSecret        string `env:"HASH_SECRET,required,notEmpty"`
	JWTPrivateKeyPath string `env:"JWT_PRIVATE_KEY_PATH,required,notEmpty"`
	JWTPublicKeyPath  string `env:"JWT_PUBLIC_KEY_PATH,required,notEmpty"`
	Issuer            string `env:"ISSUER,required,notEmpty"`

	AccessTokenTTL  time.Duration `env:"ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"720h"`

	Code CodeConfig `envPrefix:"CODE_"`
	// TwoFactor is the second step of signing in with an authenticator app.
	TwoFactor TwoFactorConfig `envPrefix:"TWO_FACTOR_"`
}

type TwoFactorConfig struct {
	// Issuer names the site in the app.
	Issuer string `env:"ISSUER" envDefault:"Freenet"`
	// TicketTTL is how long the code of the app may take after the email
	// code.
	TicketTTL time.Duration `env:"TICKET_TTL" envDefault:"10m"`
	// ChecksPerUser codes of a page are checked per ChecksWindow.
	ChecksPerUser int           `env:"CHECKS_PER_USER" envDefault:"10"`
	ChecksWindow  time.Duration `env:"CHECKS_WINDOW" envDefault:"1h"`
}

type SMTPConfig struct {
	Host     string `env:"HOST"`
	Port     string `env:"PORT"`
	Username string `env:"USERNAME"`
	Password string `env:"PASSWORD"`
	From     string `env:"FROM"`
}

type TelegramConfig struct {
	BotToken string `env:"BOT_TOKEN"`
	ChatID   string `env:"CHAT_ID"`
	// API is the Bot API; tests serve their own.
	API string `env:"API" envDefault:"https://api.telegram.org"`
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
	case MailerTelegram:
		if c.Telegram.BotToken == "" || c.Telegram.ChatID == "" || c.Telegram.API == "" {
			return errors.New("MAILER=telegram requires TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID")
		}
	default:
		return fmt.Errorf("unknown MAILER %q, want %q, %q or %q", c.Mailer, MailerConsole, MailerSMTP, MailerTelegram)
	}

	if c.Code.TTL <= 0 || c.Code.ResendInterval <= 0 || c.Code.RequestsWindow <= 0 ||
		c.Code.MaxAttempts <= 0 || c.Code.RequestsPerIP <= 0 {
		return errors.New("CODE_* settings must be positive")
	}

	if c.TwoFactor.Issuer == "" || c.TwoFactor.TicketTTL <= 0 || c.TwoFactor.ChecksPerUser <= 0 || c.TwoFactor.ChecksWindow <= 0 {
		return errors.New("TWO_FACTOR_* settings must be set and positive")
	}

	if err := c.Tracing.Validate(); err != nil {
		return err
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
