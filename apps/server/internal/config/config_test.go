package config

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/caarlos0/env/v11"
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
		{"BRAND_NAME": " "},
		{"BRAND_LOGO_URL": "javascript:alert(1)"},
		{"BRAND_LOGO_URL": "//example.com/logo.png"},
		{"BRAND_LOGO_URL": "/\\example.com/logo.png"},
		{"BRAND_LOGO_URL": "https://user:password@example.com/logo.png"},
	} {
		vars["AUTH_DISABLED"] = "true"
		if _, err := parse(env.Options{Environment: vars}); err == nil {
			t.Errorf("accepted invalid config: %v", vars)
		}
	}
}

func TestBranding(t *testing.T) {
	for _, logo := range []string{"", "/logo.png", "https://example.com/logo.png"} {
		cfg, err := parse(env.Options{Environment: map[string]string{"AUTH_DISABLED": "true", "COMPANY_NAME": " Example Company ", "BRAND_NAME": "Example Finance", "BRAND_LOGO_URL": logo}})
		if err != nil || cfg.Branding.CompanyName != "Example Company" || cfg.Branding.Name != "Example Finance" || cfg.Branding.LogoURL != logo {
			t.Fatalf("branding was not loaded: %v", err)
		}
	}
}
