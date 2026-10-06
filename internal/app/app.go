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

	"github.com/SuperrNaruto/Crychic/internal/bangumi"
	"github.com/SuperrNaruto/Crychic/internal/config"
	"github.com/SuperrNaruto/Crychic/internal/flow"
	"github.com/SuperrNaruto/Crychic/internal/moviepilot"
	"github.com/SuperrNaruto/Crychic/internal/notify"
	"github.com/SuperrNaruto/Crychic/internal/telegram"
)

const (
	// backendTimeout bounds every MoviePilot and Bangumi call; searches hit
	// TMDB upstream.
	backendTimeout  = 30 * time.Second
	telegramTimeout = 70 * time.Second // longer than Telegram's one-minute long poll
	imageTimeout    = 10 * time.Second
	// stateFile holds requests awaiting arrival, inside the data dir.
	stateFile = "requests.json"
)

// Deps are what the app takes from its host besides configuration.
type Deps struct {
	Log *slog.Logger
	Now func() time.Time
	// Images downloads posters chat platforms cannot fetch themselves;
	// nil uses a plain client.
	Images *http.Client
}

// Run serves the Telegram bot and arrival notices until ctx is cancelled.
func Run(ctx context.Context, cfg config.Config, deps Deps) error {
	log, now := deps.Log, deps.Now
	transport := newTransport()
	images := deps.Images
	if images == nil {
		images = &http.Client{Timeout: imageTimeout, Transport: transport}
	}
	backend := moviepilot.New(cfg.MoviePilotURL, cfg.MoviePilotAPIKey, &http.Client{Timeout: backendTimeout, Transport: transport})
	tg, err := telegram.New(telegram.Config{
		Token:        cfg.TelegramToken,
		APIURL:       cfg.TelegramAPIURL,
		AllowedUsers: cfg.TelegramAllowed,
		FollowEvery:  cfg.FollowEvery,
		HTTPClient:   &http.Client{Timeout: telegramTimeout, Transport: transport},
		ImageClient:  images,
		Log:          log,
	})
	if err != nil {
		return err
	}
	notifier, err := notify.New(notify.Options{
		Feed:        backend,
		Sender:      tg,
		Path:        filepath.Join(cfg.DataDir, stateFile),
		Interval:    cfg.NotifyInterval,
		Quiet:       cfg.NotifyQuiet,
		Destination: cfg.TelegramNotifyChat,
		LibraryWait: cfg.LibraryWait,
		Now:         now,
		Log:         log,
	})
	if err != nil {
		return err
	}
	calendar := bangumi.New(cfg.BangumiAPIURL, &http.Client{Timeout: backendTimeout, Transport: transport})
	engine := flow.New(flow.Options{
		Backend: backend, Calendar: calendar, Watcher: notifier, Log: log, Now: now,
		NoticeInChannel: cfg.TelegramNotifyChat != "",
	})
	var wg sync.WaitGroup
	wg.Go(func() { notifier.Run(ctx) })
	tg.Run(ctx, engine)
	wg.Wait()
	return nil
}
