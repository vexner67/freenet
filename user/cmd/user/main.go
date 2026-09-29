package main

import (
	"context"
	"fmt"
	"os"

	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/user/internal/app"
	"github.com/vexner67/freenet/user/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config.Load", err)
		os.Exit(1)
	}

	log, err := logger.New(logger.Config{
		LogLevel:  cfg.LogLevel,
		LogFormat: cfg.LogFormat,
	})
	if err != nil {
		fmt.Println("logger.New", err)
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
