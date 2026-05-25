package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/nearai/ironclaw-go/internal/app"
	"github.com/nearai/ironclaw-go/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	application, err := app.Build(cfg)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return application.Run(ctx)
}
