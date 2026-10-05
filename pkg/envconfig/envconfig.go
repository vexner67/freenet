// Package envconfig loads service configuration from the environment.
package envconfig

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Validator checks rules that span several variables, e.g. "persistent
// storage needs DATABASE_URL".
type Validator interface {
	Validate() error
}

// Load reads .env if present, then the environment. Variables already set
// in the environment win over .env, so containers need no .env at all.
func Load[T Validator]() (T, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		var zero T
		return zero, fmt.Errorf("godotenv.Load: %w", err)
	}

	return Parse[T](env.Options{})
}

// Parse fills T from opts (the process environment by default) and
// validates it. Tests pass opts.Environment instead of touching os.Environ.
func Parse[T Validator](opts env.Options) (T, error) {
	cfg, err := env.ParseAsWithOptions[T](opts)
	if err != nil {
		return cfg, fmt.Errorf("env.ParseAsWithOptions: %w", err)
	}

	if err = cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("cfg.Validate: %w", err)
	}

	return cfg, nil
}
