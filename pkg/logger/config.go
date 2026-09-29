package logger

import (
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/vexner67/freenet/pkg/errs"
)

type Config struct {
	LogLevel  string `env:"LOG_LEVEL,required"`
	LogFormat string `env:"LOG_FORMAT,required"`
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil {
		return Config{}, errs.Wrap("godotenv.Load", err)
	}

	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, errs.Wrap("env.ParseAs", err)
	}

	return cfg, nil
}
