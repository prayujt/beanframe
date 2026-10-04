package notifications

import (
	"fmt"
	"net/url"
	"strings"
)

type Config struct {
	DiscordURL string `env:"DISCORD_WEBHOOK_URL"`
	SlackURL   string `env:"SLACK_WEBHOOK_URL"`
	JSONURL    string `env:"JSON_WEBHOOK_URL"`
	Events     string `env:"WEBHOOK_EVENTS" envDefault:"created"`
}

func (c *Config) Validate() error {
	c.Events = strings.TrimSpace(c.Events)
	if c.Events == "" {
		c.Events = "created"
	}
	if c.Events != "created" && c.Events != "transactions" && c.Events != "all" {
		return fmt.Errorf("WEBHOOK_EVENTS must be created, transactions, or all")
	}
	for _, setting := range []struct {
		name  string
		value *string
	}{
		{"DISCORD_WEBHOOK_URL", &c.DiscordURL},
		{"SLACK_WEBHOOK_URL", &c.SlackURL},
		{"JSON_WEBHOOK_URL", &c.JSONURL},
	} {
		*setting.value = strings.TrimSpace(*setting.value)
		if *setting.value == "" {
			continue
		}
		u, err := url.Parse(*setting.value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(*setting.value, "\\\r\n\t") {
			return fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials or a fragment", setting.name)
		}
	}
	return nil
}
