package config

import (
	"errors"
	"time"

	"github.com/vexner67/freenet/pkg/envconfig"
	"github.com/vexner67/freenet/pkg/mtls"
)

type Config struct {
	HTTPPort  int    `env:"HTTP_PORT,required,notEmpty"`
	LogLevel  string `env:"LOG_LEVEL,required,notEmpty"`
	LogFormat string `env:"LOG_FORMAT,required,notEmpty"`

	AuthServiceAddr  string `env:"AUTH_SERVICE_ADDR,required,notEmpty"`
	UserServiceAddr  string `env:"USER_SERVICE_ADDR,required,notEmpty"`
	PostServiceAddr  string `env:"POST_SERVICE_ADDR,required,notEmpty"`
	MediaServiceAddr string `env:"MEDIA_SERVICE_ADDR,required,notEmpty"`

	// mTLS to the internal services: TLS_CA_PATH, TLS_CERT_PATH, TLS_KEY_PATH.
	TLS mtls.Config `envPrefix:"TLS_"`

	// CORSAllowedOrigins are the web frontends allowed to call the API,
	// e.g. "http://localhost:5173,https://freenet.example".
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
	// TrustForwardedFor takes the client address from X-Forwarded-For;
	// enable only behind a proxy that sets it.
	TrustForwardedFor bool          `env:"TRUST_FORWARDED_FOR" envDefault:"false"`
	RequestTimeout    time.Duration `env:"REQUEST_TIMEOUT" envDefault:"10s"`

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

	return c.TLS.Validate()
}
