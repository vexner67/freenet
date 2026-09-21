package main

import (
	"context"
	"os"

	"github.com/vexner67/freenet/auth/internal/app"
	"github.com/vexner67/freenet/auth/internal/config"
	"github.com/vexner67/freenet/auth/internal/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		os.Exit(1)
	}

	log, err := logger.New(cfg)
	if err != nil {
		os.Exit(1)
	}

	a, err := app.New(cfg)
	if err != nil {
		log.Error("app.New", err)
		os.Exit(1)
	}

	if err = a.Run(context.Background()); err != nil {
		log.Error("a.Run", err)
		os.Exit(1)
	}
}
