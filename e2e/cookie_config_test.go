package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise main's real configuration/startup boundary. Invalid secrets are
// rejected before any network work and never echoed in process output.
func TestResourceCookieConfigurationRejectsUnsafeInput(t *testing.T) {
	binary := buildStartupBinary(t)
	h := start(t, scenario{})
	cases := []struct{ name, cookie, origin string }{
		{"Cookie header", "MoviePilot=" + resourceCookie, h.mp.URL},
		{"extra Cookie", resourceCookie + "; Other=synthetic", h.mp.URL},
		{"newline", resourceCookie + "\r\nX-Test: synthetic", h.mp.URL},
		{"whitespace", " " + resourceCookie, h.mp.URL},
		{"quoted value", `"` + resourceCookie + `"`, h.mp.URL},
		{"non-ASCII", resourceCookie + "中文", h.mp.URL},
		{"public cleartext", resourceCookie, "http://moviepilot.example.com"},
		{"URL credentials", resourceCookie, "https://synthetic:credential@moviepilot.example.com"},
		{"URL query", resourceCookie, "https://moviepilot.example.com?token=synthetic"},
	}
	for _, tc := range cases {
		out, err := rejectedStartup(binary, tc.cookie, tc.origin)
		if err == nil || !strings.Contains(out, "CRYCHIC_MOVIEPILOT_COOKIE") {
			t.Fatalf("%s did not reject configuration before startup", tc.name)
		}
		if strings.Contains(out, resourceCookie) || strings.Contains(out, tc.cookie) {
			t.Fatalf("%s echoed a credential in the startup error", tc.name)
		}
		h.tr.add(fmt.Sprintf(">> startup rejects %s without echoing credentials", tc.name))
	}
	h.say(alice, alice, "/help") // unset remains a fully usable deployment
	h.tr.verify(t)
}

func buildStartupBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crychic")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "../cmd/crychic")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build startup binary: %v\n%s", err, out)
	}
	return path
}

func rejectedStartup(binary, cookie, origin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "CRYCHIC_") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env,
		"CRYCHIC_MOVIEPILOT_URL="+origin, "CRYCHIC_MOVIEPILOT_COOKIE="+cookie,
		"CRYCHIC_MOVIEPILOT_API_KEY="+mpAPIKey, "CRYCHIC_TELEGRAM_TOKEN="+tgToken,
		"CRYCHIC_TELEGRAM_ALLOWED_USERS=1001", "CRYCHIC_TELEGRAM_API_URL=http://127.0.0.1:1",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
