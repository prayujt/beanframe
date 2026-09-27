// Package config loads and validates the application's environment configuration.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/prayujt/beanframe/apps/server/internal/auth"
)

type Branding struct {
	CompanyName string `env:"COMPANY_NAME"`
	Name        string `env:"BRAND_NAME" envDefault:"Beanframe"`
	LogoURL     string `env:"BRAND_LOGO_URL"`
}

type Config struct {
	Branding      Branding
	LedgerSeedDir string `env:"LEDGER_SEED_DIR"`
	Auth          auth.Config
	LogLevel      slog.Level `env:"LOG_LEVEL" envDefault:"debug"`
	LogFormat     string     `env:"LOG_FORMAT" envDefault:"text"`
	ListenAddr    string     `env:"LISTEN_ADDR" envDefault:":8080"`
	WebDir        string     `env:"WEB_DIR" envDefault:"apps/web/build"`
	LedgerPath    string     `env:"LEDGER_PATH" envDefault:"testdata/ledger/main.beancount"`
	Python        string     `env:"PYTHON" envDefault:"python3"`
}

func Load() (Config, error) {
	return parse(env.Options{})
}

func parse(options env.Options) (Config, error) {
	cfg, err := env.ParseAsWithOptions[Config](options)
	if err != nil {
		return cfg, fmt.Errorf("load configuration: %w", err)
	}
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return cfg, fmt.Errorf("LOG_FORMAT must be json or text")
	}
	cfg.Branding.CompanyName = strings.TrimSpace(cfg.Branding.CompanyName)
	cfg.Branding.Name = strings.TrimSpace(cfg.Branding.Name)
	if cfg.Branding.Name == "" {
		return cfg, fmt.Errorf("BRAND_NAME must not be blank")
	}
	cfg.Branding.LogoURL = strings.TrimSpace(cfg.Branding.LogoURL)
	if value := cfg.Branding.LogoURL; value != "" {
		u, err := url.Parse(value)
		relative := strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//")
		if err != nil || strings.ContainsAny(value, "\\\r\n\t") || u.User != nil || (!relative && ((u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "")) {
			return cfg, fmt.Errorf("BRAND_LOGO_URL must be an HTTP(S) URL or a root-relative path")
		}
	}
	_, port, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil {
		return cfg, fmt.Errorf("LISTEN_ADDR must be host:port: %w", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return cfg, fmt.Errorf("LISTEN_ADDR port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.WebDir) == "" {
		return cfg, fmt.Errorf("WEB_DIR must not be blank")
	}

	if strings.TrimSpace(cfg.LedgerPath) == "" || strings.TrimSpace(cfg.Python) == "" {
		return cfg, fmt.Errorf("LEDGER_PATH and PYTHON must not be blank")
	}
	cfg.LedgerPath, err = filepath.Abs(cfg.LedgerPath)
	if err != nil {
		return cfg, fmt.Errorf("resolve LEDGER_PATH: %w", err)
	}

	if err := cfg.Auth.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}
