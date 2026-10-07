package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vexner67/freenet/message/internal/app"
	"github.com/vexner67/freenet/message/internal/config"
	"github.com/vexner67/freenet/pkg/logger"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check that the service running with this configuration serves, then exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config.Load:", err)
		os.Exit(1)
	}

	log, err := logger.New(logger.Config{
		LogLevel:  cfg.LogLevel,
		LogFormat: cfg.LogFormat,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger.New:", err)
		os.Exit(1)
	}

	// Ctrl+C locally, SIGTERM from Docker/Kubernetes: stop gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *healthcheck {
		if err = app.Healthcheck(ctx, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "app.Healthcheck:", err)
			os.Exit(1)
		}
		return
	}

	a, err := app.New(cfg)
	if err != nil {
		log.Error("app.New", "error", err)
		os.Exit(1)
	}

	err = a.Run(ctx)
	a.Close()

	if err != nil {
		log.Error("a.Run", "error", err)
		os.Exit(1)
	}
}
