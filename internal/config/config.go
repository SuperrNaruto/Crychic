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
	defaultNotifyStall    = 6 * time.Hour
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

// notifyStall is how long a requested download may make no progress before
// its requesters hear it is stuck; "0s" never tells them.
var notifyStall = durationVar{"CRYCHIC_NOTIFY_STALL", defaultNotifyStall, 0}

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
	NotifyStall        time.Duration
	FollowEvery        time.Duration
	// NotifyDigest is when the weekly arrival digest goes out; a nil Zone
	// sends none.
	NotifyDigest Digest
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
	durations := []struct {
		v  durationVar
		to *time.Duration
	}{
		{notifyInterval, &cfg.NotifyInterval}, {notifyQuiet, &cfg.NotifyQuiet},
		{libraryWait, &cfg.LibraryWait}, {notifyStall, &cfg.NotifyStall}, {followEvery, &cfg.FollowEvery},
	}
	for _, d := range durations {
		var err error
		if *d.to, err = d.v.parse(getenv); err != nil {
			errs = append(errs, err)
		}
	}
	var err error
	if cfg.NotifyDigest, err = parseDigest(getenv("CRYCHIC_NOTIFY_DIGEST")); err != nil {
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

// Digest is when the weekly arrival digest goes out: Day at At past
// midnight in Zone (China time); a nil Zone sends none.
type Digest struct {
	Day  time.Weekday
	At   time.Duration
	Zone *time.Location
}

const chinaOffset = 8 * 60 * 60

// digestZone is the time zone digest moments are read in.
var digestZone = time.FixedZone("UTC+8", chinaOffset)

var digestDays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// parseDigest reads CRYCHIC_NOTIFY_DIGEST, e.g. "sun 20:00"; empty sends
// no digest.
func parseDigest(raw string) (Digest, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Digest{}, nil
	}
	day, clock, _ := strings.Cut(strings.ToLower(raw), " ")
	weekday, okDay := digestDays[day]
	at, err := time.Parse("15:04", strings.TrimSpace(clock))
	if !okDay || err != nil {
		return Digest{}, fmt.Errorf("CRYCHIC_NOTIFY_DIGEST: want a weekday and time like \"sun 20:00\", got %q", raw)
	}
	offset := time.Duration(at.Hour())*time.Hour + time.Duration(at.Minute())*time.Minute
	return Digest{Day: weekday, At: offset, Zone: digestZone}, nil
}
