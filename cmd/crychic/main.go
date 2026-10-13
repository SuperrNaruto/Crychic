package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/app"
	"github.com/SuperrNaruto/Crychic/internal/config"
)

// version is stamped by -ldflags "-X main.version=..." where the build has no
// git metadata (the Docker context excludes .git).
var version string

// develVersion is what the toolchain reports when it found no VCS information.
const develVersion = "(devel)"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	v := buildVersion()
	if *showVersion {
		fmt.Println(v)
		return
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	log.Info("crychic starting", "version", v)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, app.Deps{Log: log, Now: time.Now, CalendarWarm: true}); err != nil {
		log.Error("crychic stopped", "err", err)
		os.Exit(1)
	}
}

// buildVersion prefers the stamped version, then the git tag or pseudo-version
// that go build (Go 1.24+) records from the repository.
func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" && info.Main.Version != develVersion {
		return info.Main.Version
	}
	return "dev"
}
