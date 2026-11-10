package config

import (
	"errors"
	"net/url"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/tracing"
)

const (
	StorageInmem      = "inmem"
	StoragePersistent = "persistent"
)

type Config struct {
	GRPCPort            int             `env:"GRPC_PORT" envDefault:"7059"`
	MetricsPort         int             `env:"METRICS_PORT" envDefault:"9059"`
	LogLevel            string          `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat           string          `env:"LOG_FORMAT" envDefault:"text"`
	TLS                 mtls.Config     `envPrefix:"TLS_"`
	Tracing             tracing.Config  `envPrefix:"TRACING_"`
	Storage             string          `env:"STORAGE" envDefault:"inmem"`
	DatabaseReadTimeout time.Duration   `env:"DATABASE_READ_TIMEOUT" envDefault:"2s"`
	DatabaseURL         string          `env:"DATABASE_URL"`
	RedisAddr           string          `env:"REDIS_ADDR"`
	MigrateOnStart      bool            `env:"MIGRATE_ON_START" envDefault:"true"`
	UserAddr            string          `env:"USER_SERVICE_ADDR" envDefault:"localhost:7052"`
	ClubAddr            string          `env:"CLUB_SERVICE_ADDR" envDefault:"localhost:7057"`
	NotificationAddr    string          `env:"NOTIFICATION_SERVICE_ADDR" envDefault:"localhost:7055"`
	ServicesTimeout     time.Duration   `env:"SERVICES_TIMEOUT" envDefault:"2s"`
	ConnectTimeout      time.Duration   `env:"CONNECT_TIMEOUT" envDefault:"5s"`
	MigrationTimeout    time.Duration   `env:"MIGRATION_TIMEOUT" envDefault:"1m"`
	JWTPublicKeyPath    string          `env:"JWT_PUBLIC_KEY_PATH,required,notEmpty"`
	JWTIssuer           string          `env:"JWT_ISSUER,required,notEmpty"`
	LiveKitURL          string          `env:"LIVEKIT_URL" envDefault:"http://localhost:7880"`
	LiveKitPublicURL    string          `env:"LIVEKIT_PUBLIC_URL" envDefault:"ws://localhost:8080"`
	LiveKitKey          string          `env:"LIVEKIT_API_KEY,required,notEmpty"`
	LiveKitSecret       string          `env:"LIVEKIT_API_SECRET,required,notEmpty"`
	LiveKitTimeout      time.Duration   `env:"LIVEKIT_TIMEOUT" envDefault:"3s"`
	LiveKitTokenTTL     time.Duration   `env:"LIVEKIT_TOKEN_TTL" envDefault:"10m"`
	MaxMembers          int             `env:"ROOM_MAX_MEMBERS" envDefault:"5"`
	MaxLive             int             `env:"ROOM_MAX_LIVE" envDefault:"20"`
	EmptyTTL            time.Duration   `env:"ROOM_EMPTY_TTL" envDefault:"5m"`
	StateTTL            time.Duration   `env:"ROOM_STATE_TTL" envDefault:"24h"`
	SweepInterval       time.Duration   `env:"ROOM_SWEEP_INTERVAL" envDefault:"10s"`
	ProbeTimeout        time.Duration   `env:"PROBE_TIMEOUT" envDefault:"3s"`
	CreateLimits        ratelimit.Rules `env:"RATE_LIMIT_ROOMS" envDefault:"5/1h"`
	JoinLimits          ratelimit.Rules `env:"RATE_LIMIT_ROOM_JOINS" envDefault:"30/1m"`
	ChatLimits          ratelimit.Rules `env:"RATE_LIMIT_ROOM_CHAT" envDefault:"30/1m,1000/24h"`
	ScreenLimits        ratelimit.Rules `env:"RATE_LIMIT_ROOM_PLAYBACK" envDefault:"60/1m"`
	InviteLimits        ratelimit.Rules `env:"RATE_LIMIT_ROOM_INVITES" envDefault:"30/1h,100/24h"`
	ManageLimits        ratelimit.Rules `env:"RATE_LIMIT_ROOM_MANAGEMENT" envDefault:"60/1m"`
}

func Load() (Config, error) { return envconfig.Load[Config]() }
func (c Config) Validate() error {
	switch c.Storage {
	case StorageInmem:
	case StoragePersistent:
		if c.DatabaseURL == "" || c.RedisAddr == "" {
			return errors.New("persistent rooms require DATABASE_URL and REDIS_ADDR")
		}
		if !c.TLS.Enabled() {
			return errors.New("persistent rooms require mTLS")
		}
	default:
		return errors.New("invalid STORAGE")
	}
	if c.GRPCPort < 1 || c.GRPCPort >= 32768 || c.MetricsPort < 0 || c.MetricsPort >= 32768 || c.MaxMembers < 1 || c.MaxMembers > 100 || c.MaxLive < 1 || c.MaxLive > 10000 {
		return errors.New("invalid room capacity or port")
	}
	for _, d := range []time.Duration{c.DatabaseReadTimeout, c.ServicesTimeout, c.ConnectTimeout, c.MigrationTimeout, c.LiveKitTimeout, c.LiveKitTokenTTL, c.EmptyTTL, c.SweepInterval, c.ProbeTimeout} {
		if d <= 0 {
			return errors.New("room timeouts must be positive")
		}
	}
	if c.StateTTL < c.EmptyTTL+time.Minute || c.SweepInterval > 15*time.Second {
		return errors.New("room state TTL is too short or sweep interval too long")
	}
	for _, item := range []struct {
		raw       string
		websocket bool
	}{{c.LiveKitURL, false}, {c.LiveKitPublicURL, true}} {
		u, err := url.Parse(item.raw)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid LiveKit URL")
		}
		if item.websocket && u.Scheme != "ws" && u.Scheme != "wss" {
			return errors.New("LIVEKIT_PUBLIC_URL must use ws or wss")
		}
		if !item.websocket && u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("LIVEKIT_URL must use http or https")
		}
	}
	if err := c.Tracing.Validate(); err != nil {
		return err
	}
	return c.TLS.Validate()
}
