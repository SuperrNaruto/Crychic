package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/app"
	"github.com/SuperrNaruto/Crychic/internal/config"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, app.Deps{Log: log, Now: time.Now}); err != nil {
		log.Error("crychic stopped", "err", err)
		os.Exit(1)
	}
}
