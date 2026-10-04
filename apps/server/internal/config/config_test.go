package config

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/prayujt/beanframe/apps/server/internal/notifications"
)

func TestDefaults(t *testing.T) {
	cfg, err := parse(env.Options{Environment: map[string]string{"AUTH_DISABLED": "true"}})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs("testdata/ledger/main.beancount")
	if cfg.LogLevel != slog.LevelDebug || cfg.LogFormat != "text" || cfg.ListenAddr != ":8080" || cfg.WebDir != "apps/web/build" || cfg.LedgerPath != want || cfg.Python != "python3" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.Webhooks != (notifications.Config{Events: "created"}) {
		t.Fatal("webhooks must be disabled by default")
	}
}

func TestWebhookConfiguration(t *testing.T) {
	for _, name := range []string{"DISCORD_WEBHOOK_URL", "SLACK_WEBHOOK_URL", "JSON_WEBHOOK_URL"} {
		for _, value := range []string{"", "  ", " https://example.com/hook?token=secret ", "http://localhost:1234/hook"} {
			cfg, err := parse(env.Options{Environment: map[string]string{"AUTH_DISABLED": "true", name: value}})
			if err != nil {
				t.Fatalf("%s rejected %q: %v", name, value, err)
			}
			got := map[string]string{"DISCORD_WEBHOOK_URL": cfg.Webhooks.DiscordURL, "SLACK_WEBHOOK_URL": cfg.Webhooks.SlackURL, "JSON_WEBHOOK_URL": cfg.Webhooks.JSONURL}[name]
			if got != strings.TrimSpace(value) {
				t.Fatalf("%s = %q", name, got)
			}
		}
		for _, value := range []string{"/relative", "ftp://example.com/hook", "https://", "https://user:secret@example.com/hook", "https://example.com/hook#secret", "https://example.com/\\secret", "https://example.com/secret\nhook"} {
			_, err := parse(env.Options{Environment: map[string]string{"AUTH_DISABLED": "true", name: value}})
			if err == nil || !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "secret") {
				t.Errorf("%s must reject invalid URLs without exposing secrets: %v", name, err)
			}
		}
	}
}

func TestWebhookEvents(t *testing.T) {
	for _, mode := range []string{"", "created", "transactions", "all"} {
		cfg, err := parse(env.Options{Environment: map[string]string{"AUTH_DISABLED": "true", "WEBHOOK_EVENTS": mode}})
		if mode == "" {
			mode = "created"
		}
		if err != nil || cfg.Webhooks.Events != mode {
			t.Fatalf("WEBHOOK_EVENTS %s: %+v %v", mode, cfg.Webhooks, err)
		}
	}
	_, err := parse(env.Options{Environment: map[string]string{"AUTH_DISABLED": "true", "WEBHOOK_EVENTS": "unknown"}})
	if err == nil || !strings.Contains(err.Error(), "WEBHOOK_EVENTS") {
		t.Fatal("invalid webhook event selection must fail at startup")
	}
}

func TestLoadEnvironment(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LEDGER_PATH", "example/main.beancount")
	t.Setenv("PYTHON", "/opt/venv/bin/python")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs("example/main.beancount")
	if cfg.LogLevel != slog.LevelInfo || cfg.LogFormat != "json" || cfg.LedgerPath != want || cfg.Python != "/opt/venv/bin/python" {
		t.Fatalf("environment overrides not applied: %+v", cfg)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, vars := range []map[string]string{
		{"LOG_LEVEL": "unknown"},
		{"LOG_FORMAT": "unknown"},
		{"LISTEN_ADDR": "8080"},
		{"LISTEN_ADDR": ":invalid"},
		{"LISTEN_ADDR": ":70000"},
		{"WEB_DIR": " "},
		{"LEDGER_PATH": " "},
		{"PYTHON": " "},
		{"COMPANY_LOGO_URL": "javascript:alert(1)"},
		{"COMPANY_LOGO_URL": "//example.com/logo.png"},
		{"COMPANY_LOGO_URL": "/\\example.com/logo.png"},
		{"COMPANY_LOGO_URL": "https://user:password@example.com/logo.png"},
	} {
		vars["AUTH_DISABLED"] = "true"
		if _, err := parse(env.Options{Environment: vars}); err == nil {
			t.Errorf("accepted invalid config: %v", vars)
		}
	}
}

func TestBranding(t *testing.T) {
	for _, company := range []struct{ input, name string }{
		{"", "Beanframe"},
		{"   ", "Beanframe"},
		{" Example Company ", "Example Company Beanframe"},
	} {
		for _, logo := range []string{"", "/logo.png", "https://example.com/logo.png"} {
			cfg, err := parse(env.Options{Environment: map[string]string{"AUTH_DISABLED": "true", "COMPANY_NAME": company.input, "COMPANY_LOGO_URL": logo}})
			if err != nil || cfg.Branding.Name != company.name || cfg.Branding.LogoURL != logo {
				t.Fatalf("unexpected branding for %q: %+v, %v", company.input, cfg.Branding, err)
			}
		}
	}
}
