// Package auth implements native OIDC browser sessions and OAuth-protected MCP.
package auth

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	Disabled     bool          `env:"AUTH_DISABLED" envDefault:"false"`
	PublicURL    string        `env:"PUBLIC_URL"`
	IssuerURL    string        `env:"OIDC_ISSUER_URL"`
	ClientID     string        `env:"OIDC_CLIENT_ID"`
	ClientSecret string        `env:"OIDC_CLIENT_SECRET"`
	ProviderName string        `env:"OIDC_PROVIDER_NAME" envDefault:"OpenID Connect"`
	MCPIssuerURL string        `env:"MCP_ISSUER_URL"`
	SessionTTL   time.Duration `env:"SESSION_TTL" envDefault:"15m"`
}

func (c *Config) Validate() error {
	if c.Disabled {
		if c.IssuerURL != "" || c.ClientID != "" || c.ClientSecret != "" || c.MCPIssuerURL != "" {
			return fmt.Errorf("AUTH_DISABLED cannot be combined with OIDC credentials")
		}
		return nil
	}
	if c.ClientID == "" {
		return fmt.Errorf("OIDC_CLIENT_ID is required (AUTH_DISABLED=true is only for local sample-ledger development)")
	}
	for name, value := range map[string]string{"PUBLIC_URL": c.PublicURL, "OIDC_ISSUER_URL": c.IssuerURL} {
		if err := validURL(value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	u, _ := url.Parse(c.PublicURL)
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("PUBLIC_URL must be an origin without a path")
	}
	c.PublicURL = strings.TrimSuffix(c.PublicURL, "/")
	if c.MCPIssuerURL == "" {
		c.MCPIssuerURL = c.IssuerURL
	}
	if err := validURL(c.MCPIssuerURL); err != nil {
		return fmt.Errorf("MCP_ISSUER_URL: %w", err)
	}
	if c.SessionTTL < time.Minute || c.SessionTTL > time.Hour {
		return fmt.Errorf("SESSION_TTL must be between 1m and 1h")
	}
	return nil
}

func validURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return fmt.Errorf("HTTPS is required except on loopback")
	}
	return nil
}
