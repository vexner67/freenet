package config

import (
	"errors"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/tracing"
)

type Config struct {
	HTTPPort  int    `env:"HTTP_PORT,required,notEmpty"`
	LogLevel  string `env:"LOG_LEVEL,required,notEmpty"`
	LogFormat string `env:"LOG_FORMAT,required,notEmpty"`

	// MetricsPort serves Prometheus metrics on a port of its own, not to
	// be published outside; 0 does not serve them.
	MetricsPort int `env:"METRICS_PORT"`

	AuthServiceAddr  string `env:"AUTH_SERVICE_ADDR,required,notEmpty"`
	UserServiceAddr  string `env:"USER_SERVICE_ADDR,required,notEmpty"`
	PostServiceAddr  string `env:"POST_SERVICE_ADDR,required,notEmpty"`
	MediaServiceAddr string `env:"MEDIA_SERVICE_ADDR,required,notEmpty"`
	// The notifications of the signed-in user: the menu counter and the
	// "Мои Ответы" page.
	NotificationServiceAddr string `env:"NOTIFICATION_SERVICE_ADDR,required,notEmpty"`
	MessageServiceAddr      string `env:"MESSAGE_SERVICE_ADDR,required,notEmpty"`
	ClubServiceAddr         string `env:"CLUB_SERVICE_ADDR,required,notEmpty"`
	WorldServiceAddr        string `env:"WORLD_SERVICE_ADDR,required,notEmpty"`

	// mTLS to the internal services: TLS_CA_PATH, TLS_CERT_PATH, TLS_KEY_PATH.
	TLS mtls.Config `envPrefix:"TLS_"`

	// Tracing of requests through the services: TRACING_ENDPOINT, the
	// collector (without it nothing is traced), TRACING_SAMPLE_RATIO.
	Tracing tracing.Config `envPrefix:"TRACING_"`

	// CORSAllowedOrigins are the web frontends allowed to call the API,
	// e.g. "http://localhost:5173,https://freenet.example".
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
	// TrustForwardedFor takes the client address from X-Forwarded-For and
	// the request ID from X-Request-ID; enable only behind a proxy that
	// sets them.
	TrustForwardedFor bool          `env:"TRUST_FORWARDED_FOR" envDefault:"false"`
	RequestTimeout    time.Duration `env:"REQUEST_TIMEOUT" envDefault:"10s"`

	// StatisticsTTL is how long the gateway keeps the numbers of the page
	// "о сайте": each is a count over a whole table of its service.
	StatisticsTTL time.Duration `env:"STATISTICS_TTL" envDefault:"1m"`

	// LinkPreviewTimeout is the most the gateway spends reading the page a
	// new post links to, for its card; 0 makes posts without cards.
	LinkPreviewTimeout time.Duration `env:"LINK_PREVIEW_TIMEOUT" envDefault:"4s"`

	// WebDir is the website (the web/ directory) to serve on the same
	// origin as the API; empty serves the API only.
	WebDir string `env:"WEB_DIR"`
}

func Load() (Config, error) {
	return envconfig.Load[Config]()
}

func (c Config) Validate() error {
	if c.RequestTimeout <= 0 {
		return errors.New("REQUEST_TIMEOUT must be positive")
	}

	if c.LinkPreviewTimeout < 0 || c.LinkPreviewTimeout >= c.RequestTimeout {
		return errors.New("LINK_PREVIEW_TIMEOUT must be at least 0 and shorter than REQUEST_TIMEOUT")
	}

	if c.StatisticsTTL < 0 {
		return errors.New("STATISTICS_TTL must not be negative")
	}

	if err := c.Tracing.Validate(); err != nil {
		return err
	}

	return c.TLS.Validate()
}
