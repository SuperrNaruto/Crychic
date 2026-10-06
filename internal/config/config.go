// Package config reads Crychic's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"regexp"
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
	defaultNotifyQuiet    = 3 * time.Minute
	defaultLibraryWait    = 30 * time.Minute
	defaultFollowEvery    = 5 * time.Second
	minimumFollowEvery    = 100 * time.Millisecond
)

// libraryWait is how long a notice waits for the media server to show
// what arrived; "0s" sends as soon as the transfer is seen.
var libraryWait = durationVar{"CRYCHIC_NOTIFY_LIBRARY_WAIT", defaultLibraryWait, 0}

// followEvery is how often a live task view refreshes; Telegram limits how
// often one message may be edited, so seconds rather than milliseconds.
var followEvery = durationVar{"CRYCHIC_PROGRESS_INTERVAL", defaultFollowEvery, minimumFollowEvery}

// notifyInterval is how often arrivals are checked, e.g. "1m" or "30s".
var notifyInterval = durationVar{"CRYCHIC_NOTIFY_INTERVAL", defaultNotifyInterval, minimumNotifyInterval}

// notifyQuiet is how long a show's arrivals settle before one notice
// covers them all; "0s" announces every poll's arrivals right away.
var notifyQuiet = durationVar{"CRYCHIC_NOTIFY_QUIET", defaultNotifyQuiet, 0}

// Config is the full runtime configuration.
type Config struct {
	MoviePilotURL      string
	MoviePilotAPIKey   string
	TelegramToken      string
	TelegramAPIURL     string
	TelegramAllowed    []int64
	TelegramNotifyChat string
	BangumiAPIURL      string
	DataDir            string
	NotifyInterval     time.Duration
	NotifyQuiet        time.Duration
	LibraryWait        time.Duration
	FollowEvery        time.Duration
}

// Load builds a Config from getenv (os.Getenv in production), reporting
// every missing or malformed variable at once.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		MoviePilotURL:      getenv("CRYCHIC_MOVIEPILOT_URL"),
		MoviePilotAPIKey:   getenv("CRYCHIC_MOVIEPILOT_API_KEY"),
		TelegramToken:      getenv("CRYCHIC_TELEGRAM_TOKEN"),
		TelegramAPIURL:     getenv("CRYCHIC_TELEGRAM_API_URL"),
		TelegramNotifyChat: strings.TrimSpace(getenv("CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID")),
	}
	if cfg.TelegramAPIURL == "" {
		cfg.TelegramAPIURL = defaultTelegramAPI
	}
	cfg.BangumiAPIURL = getenv("CRYCHIC_BANGUMI_API_URL")
	if cfg.BangumiAPIURL == "" {
		cfg.BangumiAPIURL = defaultBangumiAPI
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
	if cfg.LibraryWait, err = libraryWait.parse(getenv); err != nil {
		errs = append(errs, err)
	}
	if cfg.FollowEvery, err = followEvery.parse(getenv); err != nil {
		errs = append(errs, err)
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
	if !validNotifyChat(cfg.TelegramNotifyChat) {
		errs = append(errs, errors.New("CRYCHIC_TELEGRAM_NOTIFY_CHAT_ID: want a negative chat ID or @channelusername"))
	}
	return errors.Join(errs...)
}

var channelUsername = regexp.MustCompile(`^@[A-Za-z0-9_]+$`)

func validNotifyChat(raw string) bool {
	if raw == "" || channelUsername.MatchString(raw) {
		return true
	}
	const decimal, int64Bits = 10, 64
	id, err := strconv.ParseInt(raw, decimal, int64Bits)
	return err == nil && id < 0
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
