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
	// backendTimeout bounds every Bangumi call; MoviePilot calls are
	// bounded per call by its client, since resource searches take longer.
	backendTimeout  = 30 * time.Second
	telegramTimeout = 70 * time.Second // longer than Telegram's one-minute long poll
	imageTimeout    = 10 * time.Second
)

// Deps are what the app takes from its host besides configuration.
type Deps struct {
	Log *slog.Logger
	Now func() time.Time
	// Images downloads posters chat platforms cannot fetch themselves;
	// nil uses a plain client.
	Images *http.Client
	// AnswerLead is how long a button press's edit runs before the press is
	// answered alongside it; zero uses the bot's default.
	AnswerLead time.Duration
	// SubsGrace is how long search results wait for the subscription list
	// behind their marks; zero uses the flow's default.
	SubsGrace time.Duration
}

// Run serves the Telegram bot and native notices until ctx is cancelled.
func Run(ctx context.Context, cfg config.Config, deps Deps) error {
	log, now := deps.Log, deps.Now
	transport := newTransport()
	backend := moviePilot(cfg, transport)
	tg, err := telegramBot(cfg, transport, deps)
	if err != nil {
		return err
	}
	native, err := nativeNotifications(cfg, notify.NativeOptions{Feed: backend, Sender: tg, Log: log})
	if err != nil {
		return err
	}
	warnRetiredNotify(cfg, log)
	calendar := bangumi.New(cfg.BangumiAPIURL, &http.Client{Timeout: backendTimeout, Transport: transport})
	engine := flow.New(flow.Options{
		RunContext: ctx,
		Backend:    backend, Calendar: calendar, Log: log, Now: now, SubsGrace: deps.SubsGrace,
	})
	var wg sync.WaitGroup
	wg.Go(func() { native.Run(ctx) })
	tg.Run(ctx, engine)
	engine.Wait()
	wg.Wait()
	return nil
}

func moviePilot(cfg config.Config, transport *http.Transport) *moviepilot.Client {
	return moviepilot.New(moviepilot.Options{
		BaseURL: cfg.MoviePilotURL, APIKey: cfg.MoviePilotAPIKey, Cookie: cfg.MoviePilotCookie,
		HTTPClient: &http.Client{Transport: transport},
	})
}

func telegramBot(cfg config.Config, transport *http.Transport, deps Deps) (*telegram.Bot, error) {
	images := deps.Images
	if images == nil {
		images = &http.Client{Timeout: imageTimeout, Transport: transport}
	}
	return telegram.New(telegram.Config{
		Token: cfg.TelegramToken, APIURL: cfg.TelegramAPIURL, AllowedUsers: cfg.TelegramAllowed,
		FollowEvery: cfg.FollowEvery, AnswerLead: deps.AnswerLead, HTTPClient: &http.Client{Timeout: telegramTimeout, Transport: transport},
		ImageClient: images, Log: deps.Log,
	})
}

func nativeNotifications(cfg config.Config, opts notify.NativeOptions) (*notify.NativeConsumer, error) {
	opts.Path = filepath.Join(cfg.DataDir, "native-notifications.json")
	opts.RetryEvery = cfg.NotifyInterval
	opts.Secrets = []string{cfg.MoviePilotAPIKey, cfg.TelegramToken, cfg.MoviePilotCookie}
	return notify.NewNative(opts)
}

func warnRetiredNotify(cfg config.Config, log *slog.Logger) {
	for _, name := range cfg.RetiredNotifyVars {
		log.Warn("retired notification setting ignored", "name", name)
	}
}
