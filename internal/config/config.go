// Package config reads Crychic's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const defaultTelegramAPI = "https://api.telegram.org"

// Config is the full runtime configuration.
type Config struct {
	MoviePilotURL    string
	MoviePilotAPIKey string
	TelegramToken    string
	TelegramAPIURL   string
	TelegramAllowed  []int64
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
	allowed, err := parseIDs(getenv("CRYCHIC_TELEGRAM_ALLOWED_USERS"))
	if err != nil {
		errs = append(errs, fmt.Errorf("CRYCHIC_TELEGRAM_ALLOWED_USERS: %w", err))
	}
	cfg.TelegramAllowed = allowed
	return cfg, errors.Join(errs...)
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
