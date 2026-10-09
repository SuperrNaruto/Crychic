package config

import (
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

const maxResourceCookieBytes = 8 * 1024

// Resource tokens are unquoted base64url JWT values, not Cookie/Set-Cookie
// headers. Only syntax is checked here; MoviePilot validates purpose/identity.
var resourceCookieValue = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func (cfg Config) validateResourceCookie() error {
	if cfg.MoviePilotCookie == "" {
		return nil
	}
	if len(cfg.MoviePilotCookie) > maxResourceCookieBytes || !resourceCookieValue.MatchString(cfg.MoviePilotCookie) {
		return errors.New("CRYCHIC_MOVIEPILOT_COOKIE: want only the MoviePilot resource Cookie value, not a Cookie header")
	}
	u, err := url.Parse(cfg.MoviePilotURL)
	if err != nil || !resourceOrigin(u) {
		return errors.New("CRYCHIC_MOVIEPILOT_COOKIE: require an HTTPS MoviePilot origin or a private HTTP host, without user info, query or fragment")
	}
	return nil
}

func resourceOrigin(u *url.URL) bool {
	if !plainResourceURL(u) {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.IsPrivate() || ip.IsLoopback()
	}
	// Single-label names cover Docker service DNS; public DNS names need TLS.
	return host == "localhost" || !strings.ContainsAny(host, ".:")
}

func plainResourceURL(u *url.URL) bool {
	return u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Opaque == ""
}
