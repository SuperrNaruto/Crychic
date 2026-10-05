// Package app wires Crychic's parts together; main and the end-to-end tests
// share it so tests exercise exactly what ships.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/SuperrNauto/Crychic/internal/config"
	"github.com/SuperrNauto/Crychic/internal/flow"
	"github.com/SuperrNauto/Crychic/internal/moviepilot"
	"github.com/SuperrNauto/Crychic/internal/notify"
	"github.com/SuperrNauto/Crychic/internal/telegram"
)

const (
	// backendTimeout bounds every MoviePilot call; searches hit TMDB upstream.
	backendTimeout = 30 * time.Second
	// stateFile holds requests awaiting arrival, inside the data dir.
	stateFile = "requests.json"
)

// Run serves the Telegram bot and arrival notices until ctx is cancelled.
func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	backend := moviepilot.New(cfg.MoviePilotURL, cfg.MoviePilotAPIKey, &http.Client{Timeout: backendTimeout})
	tg, err := telegram.New(telegram.Config{
		Token:        cfg.TelegramToken,
		APIURL:       cfg.TelegramAPIURL,
		AllowedUsers: cfg.TelegramAllowed,
		HTTPClient:   &http.Client{},
		Log:          log,
	})
	if err != nil {
		return err
	}
	notifier, err := notify.New(notify.Options{
		Feed:     backend,
		Sender:   tg,
		Path:     filepath.Join(cfg.DataDir, stateFile),
		Interval: cfg.NotifyInterval,
		Now:      time.Now,
		Log:      log,
	})
	if err != nil {
		return err
	}
	engine := flow.New(flow.Options{Backend: backend, Watcher: notifier, Log: log, Now: time.Now})
	var wg sync.WaitGroup
	wg.Go(func() { notifier.Run(ctx) })
	tg.Run(ctx, engine)
	wg.Wait()
	return nil
}
