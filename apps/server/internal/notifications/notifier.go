// Package notifications delivers committed ledger events to optional webhook destinations.
package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prayujt/beanframe/apps/server/internal/engine"
	"github.com/prayujt/beanframe/apps/server/pkg/log"
)

type Event struct {
	engine.MutationEvent
	OccurredAt time.Time `json:"occurred_at"`
	Revision   string    `json:"revision"`
}

type destination struct {
	name, url string
	format    func(Event) any
	queue     chan Event
}

type Notifier struct {
	ctx          context.Context
	client       *http.Client
	destinations []*destination
	events       string
}

// New starts one worker per configured destination; unset URLs disable delivery.
func New(ctx context.Context, cfg Config, name string) *Notifier {
	n := &Notifier{ctx: ctx, events: cfg.Events, client: &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	if n.events == "" {
		n.events = "created"
	}
	if cfg.DiscordURL != "" {
		u, _ := url.Parse(cfg.DiscordURL) // Configuration is validated at startup.
		query := u.Query()
		query.Set("wait", "true")
		u.RawQuery = query.Encode()
		n.destinations = append(n.destinations, &destination{name: "discord", url: u.String(), format: func(e Event) any { return discord(e, name) }})
	}
	if cfg.SlackURL != "" {
		n.destinations = append(n.destinations, &destination{name: "slack", url: cfg.SlackURL, format: func(e Event) any { return slack(e, name) }})
	}
	if cfg.JSONURL != "" {
		n.destinations = append(n.destinations, &destination{name: "json", url: cfg.JSONURL, format: func(e Event) any { return e }})
	}
	if len(n.destinations) == 0 {
		return nil
	}
	for _, d := range n.destinations {
		d.queue = make(chan Event, 128)
		go n.run(d)
	}
	return n
}

// Publish filters and queues a committed event without making the write wait for HTTP.
func (n *Notifier) Publish(mutation engine.MutationEvent, revision string) {
	if n.events == "created" && mutation.Type != "transaction.created" || n.events == "transactions" && !strings.HasPrefix(mutation.Type, "transaction.") {
		return
	}
	event := Event{MutationEvent: mutation, OccurredAt: time.Now().UTC(), Revision: revision}
	for _, d := range n.destinations {
		select {
		case <-n.ctx.Done():
			return
		case d.queue <- event:
		default:
			log.Warn("webhook queue full; notification dropped", "destination", d.name, "event_type", event.Type, "revision", revision)
		}
	}
}

func (n *Notifier) run(d *destination) {
	for {
		select {
		case <-n.ctx.Done():
			return
		case event := <-d.queue:
			if err := n.deliver(d, event); err != nil && n.ctx.Err() == nil {
				log.Warn("webhook delivery failed", "destination", d.name, "event_type", event.Type, "revision", event.Revision, "error", err)
			}
		}
	}
}

func (n *Notifier) deliver(d *destination, event Event) error {
	body, err := json.Marshal(d.format(event))
	if err != nil {
		return fmt.Errorf("encode webhook payload: %w", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(n.ctx, http.MethodPost, d.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("invalid webhook request")
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := n.client.Do(req)
		delay := time.Second << attempt
		retry := true
		// Network errors include the secret webhook URL; keep them out of logs.
		problem := fmt.Errorf("webhook request failed")
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return nil
			}
			problem = fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
			retry = response.StatusCode == 429 || response.StatusCode >= 500 && response.StatusCode < 600
			if seconds, err := strconv.ParseFloat(response.Header.Get("Retry-After"), 64); err == nil && seconds >= 0 {
				delay = time.Duration(min(seconds, 30) * float64(time.Second))
			} else if date, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
				delay = max(0, min(time.Until(date), 30*time.Second))
			}
		}
		if !retry || attempt == 2 {
			return problem
		}
		timer := time.NewTimer(delay)
		select {
		case <-n.ctx.Done():
			timer.Stop()
			return n.ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}
