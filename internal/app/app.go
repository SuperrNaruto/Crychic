// Package app wires Crychic's parts together; main and the end-to-end tests
// share it so tests exercise exactly what ships.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/SuperrNauto/Crychic/internal/config"
	"github.com/SuperrNauto/Crychic/internal/flow"
	"github.com/SuperrNauto/Crychic/internal/moviepilot"
	"github.com/SuperrNauto/Crychic/internal/telegram"
)

// backendTimeout bounds every MoviePilot call; searches hit TMDB upstream.
const backendTimeout = 30 * time.Second

// Run serves the Telegram bot until ctx is cancelled.
func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	backend := moviepilot.New(cfg.MoviePilotURL, cfg.MoviePilotAPIKey, &http.Client{Timeout: backendTimeout})
	engine := flow.New(flow.Options{Backend: backend, Log: log, Now: time.Now})
	return telegram.Run(ctx, telegram.Config{
		Token:        cfg.TelegramToken,
		APIURL:       cfg.TelegramAPIURL,
		AllowedUsers: cfg.TelegramAllowed,
		HTTPClient:   &http.Client{},
		Log:          log,
	}, engine)
}
