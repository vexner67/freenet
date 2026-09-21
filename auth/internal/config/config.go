package config

import (
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/vexner67/freenet/auth/internal/errs"
)

type Config struct {
	GRPCPort    int    `env:"GRPC_PORT,required"`
	LogLevel    string `env:"LOG_LEVEL,required"`
	LogFormat   string `env:"LOG_FORMAT,required"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	HashSecret  string `env:"HASH_SECRET,required"`
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
