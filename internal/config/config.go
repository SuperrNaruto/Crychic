// Package config reads Crychic's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTelegramAPI    = "https://api.telegram.org"
	defaultDataDir        = "data"
	defaultNotifyInterval = time.Minute
	minimumNotifyInterval = 100 * time.Millisecond
	defaultNotifyQuiet    = 3 * time.Minute
)

// notifyInterval is how often arrivals are checked, e.g. "1m" or "30s".
var notifyInterval = durationVar{"CRYCHIC_NOTIFY_INTERVAL", defaultNotifyInterval, minimumNotifyInterval}

// notifyQuiet is how long a show's arrivals settle before one notice
// covers them all; "0s" announces every poll's arrivals right away.
var notifyQuiet = durationVar{"CRYCHIC_NOTIFY_QUIET", defaultNotifyQuiet, 0}

// Config is the full runtime configuration.
type Config struct {
	MoviePilotURL    string
	MoviePilotAPIKey string
	TelegramToken    string
	TelegramAPIURL   string
	TelegramAllowed  []int64
	DataDir          string
	NotifyInterval   time.Duration
	NotifyQuiet      time.Duration
}

// Load builds a Config from getenv (os.Getenv in production), reporting
// every missing or malformed variable at once.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		MoviePilotURL:    getenv("CRYCHIC_MOVIEPILOT_URL"),
		MoviePilotAPIKey: getenv("CRYCHIC_MOVIEPILOT_API_KEY"),
		TelegramToken:    getenv("CRYCHIC_TELEGRAM_TOKEN"),
		TelegramAPIURL:   getenv("CRYCHIC_TELEGRAM_API_URL"),
	}
	if cfg.TelegramAPIURL == "" {
		cfg.TelegramAPIURL = defaultTelegramAPI
	}
	cfg.DataDir = getenv("CRYCHIC_DATA_DIR")
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir
	}
	var errs []error
	var err error
	if cfg.NotifyInterval, err = notifyInterval.parse(getenv); err != nil {
		errs = append(errs, err)
	}
	if cfg.NotifyQuiet, err = notifyQuiet.parse(getenv); err != nil {
		errs = append(errs, err)
	}
	required := []struct{ name, value string }{
		{"CRYCHIC_MOVIEPILOT_URL", cfg.MoviePilotURL},
		{"CRYCHIC_MOVIEPILOT_API_KEY", cfg.MoviePilotAPIKey},
		{"CRYCHIC_TELEGRAM_TOKEN", cfg.TelegramToken},
	}
	for _, r := range required {
		if r.value == "" {
			errs = append(errs, fmt.Errorf("%s is required", r.name))
		}
	}
	allowed, err := parseIDs(getenv("CRYCHIC_TELEGRAM_ALLOWED_USERS"))
	if err != nil {
		errs = append(errs, fmt.Errorf("CRYCHIC_TELEGRAM_ALLOWED_USERS: %w", err))
	}
	cfg.TelegramAllowed = allowed
	return cfg, errors.Join(errs...)
}

// durationVar is a duration setting with a default and a lower bound.
type durationVar struct {
	name    string
	def     time.Duration
	minimum time.Duration
}

func (v durationVar) parse(getenv func(string) string) (time.Duration, error) {
	raw := getenv(v.name)
	if raw == "" {
		return v.def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < v.minimum {
		return 0, fmt.Errorf("%s: want a duration of at least %s, got %q", v.name, v.minimum, raw)
	}
	return d, nil
}

// parseIDs parses a comma-separated whitelist. An empty whitelist is an
// error: a bot nobody may use is always a misconfiguration.
func parseIDs(raw string) ([]int64, error) {
	var ids []int64
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user id %q", field)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("at least one user id is required")
	}
	return ids, nil
}
