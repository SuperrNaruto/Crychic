// Package config reads Crychic's settings from environment variables.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTelegramAPI    = "https://api.telegram.org"
	defaultBangumiAPI     = "https://api.bgm.tv"
	defaultDataDir        = "data"
	defaultNotifyInterval = time.Minute
	minimumNotifyInterval = 100 * time.Millisecond
	defaultFollowEvery    = 5 * time.Second
	minimumFollowEvery    = 100 * time.Millisecond
)

// followEvery is how often a live task view refreshes; Telegram limits how
// often one message may be edited, so seconds rather than milliseconds.
var followEvery = durationVar{"CRYCHIC_PROGRESS_INTERVAL", defaultFollowEvery, minimumFollowEvery}

// notifyInterval is how often arrivals are checked, e.g. "1m" or "30s".
var notifyInterval = durationVar{"CRYCHIC_NOTIFY_INTERVAL", defaultNotifyInterval, minimumNotifyInterval}

// Config is the full runtime configuration.
type Config struct {
	MoviePilotURL     string
	MoviePilotAPIKey  string
	MoviePilotCookie  string
	TelegramToken     string
	TelegramAPIURL    string
	TelegramAllowed   []int64
	BangumiAPIURL     string
	DataDir           string
	NotifyInterval    time.Duration
	FollowEvery       time.Duration
	RetiredNotifyVars []string
}

// Load builds a Config from getenv (os.Getenv in production), reporting
// every missing or malformed variable at once.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		MoviePilotURL:    getenv("CRYCHIC_MOVIEPILOT_URL"),
		MoviePilotAPIKey: getenv("CRYCHIC_MOVIEPILOT_API_KEY"),
		MoviePilotCookie: getenv("CRYCHIC_MOVIEPILOT_COOKIE"),
		TelegramToken:    getenv("CRYCHIC_TELEGRAM_TOKEN"),
		TelegramAPIURL:   cmp.Or(getenv("CRYCHIC_TELEGRAM_API_URL"), defaultTelegramAPI),
		BangumiAPIURL:    cmp.Or(getenv("CRYCHIC_BANGUMI_API_URL"), defaultBangumiAPI),
		DataDir:          cmp.Or(getenv("CRYCHIC_DATA_DIR"), defaultDataDir),
	}
	var errs []error
	durations := []struct {
		v  durationVar
		to *time.Duration
	}{
		{notifyInterval, &cfg.NotifyInterval}, {followEvery, &cfg.FollowEvery},
	}
	for _, d := range durations {
		var err error
		if *d.to, err = d.v.parse(getenv); err != nil {
			errs = append(errs, err)
		}
	}
	for _, name := range []string{"CRYCHIC_NOTIFY_QUIET", "CRYCHIC_NOTIFY_LIBRARY_WAIT", "CRYCHIC_NOTIFY_STALL", "CRYCHIC_NOTIFY_DIGEST", "CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID"} {
		if getenv(name) != "" {
			cfg.RetiredNotifyVars = append(cfg.RetiredNotifyVars, name)
		}
	}

	errs = append(errs, cfg.validate(), cfg.allowedUsers(getenv("CRYCHIC_TELEGRAM_ALLOWED_USERS")))
	return cfg, errors.Join(errs...)
}

func (cfg *Config) validate() error {
	var errs []error
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

	return errors.Join(append(errs, cfg.validateResourceCookie())...)
}

func (cfg *Config) allowedUsers(raw string) error {
	allowed, err := parseIDs(raw)
	if err != nil {
		return fmt.Errorf("CRYCHIC_TELEGRAM_ALLOWED_USERS: %w", err)
	}
	cfg.TelegramAllowed = allowed
	return nil
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
