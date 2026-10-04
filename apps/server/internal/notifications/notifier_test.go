package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/prayujt/beanframe/apps/server/internal/engine"
)

func transaction() engine.Transaction {
	return engine.Transaction{
		ID: "tx-1", File: "transactions.beancount", Line: 42, Date: "2026-04-01", Flag: "*",
		Payee: "Vendor", Narration: "Hosting", Tags: []string{"business"}, Links: []string{},
		Postings: []engine.Posting{
			{Account: "Expenses:Hosting", Units: &engine.Amount{Number: "15.2500", Currency: "USD"}},
			{Account: "Assets:Checking", Units: &engine.Amount{Number: "-15.2500", Currency: "USD"}},
		},
	}
}

func mutation(kind string) engine.MutationEvent {
	tx := transaction()
	return engine.MutationEvent{Type: kind, File: tx.File, Transaction: &tx}
}

func TestDestinations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type received struct {
		path, query string
		body        json.RawMessage
	}
	requests := make(chan received, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected a JSON POST")
		}
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- received{r.URL.Path, r.URL.RawQuery, body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	n := New(ctx, Config{DiscordURL: server.URL + "/discord?thread_id=123", SlackURL: server.URL + "/slack", JSONURL: server.URL + "/json"}, "Example Beanframe")
	n.Publish(mutation("transaction.created"), "revision-1")
	var payloads = make(map[string]received)
	for range 3 {
		select {
		case req := <-requests:
			payloads[req.path] = req
		case <-time.After(3 * time.Second):
			t.Fatal("missing webhook delivery")
		}
	}
	var event Event
	if err := json.Unmarshal(payloads["/json"].body, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "transaction.created" || event.Revision != "revision-1" || event.OccurredAt.IsZero() || event.Transaction.ID != "tx-1" || event.Transaction.Postings[0].Units.Number != "15.2500" {
		t.Fatalf("incomplete generic event: %+v", event)
	}
	var discordBody struct {
		Embeds []struct {
			Title, Description string
			Color              int
			Fields             []struct{ Name, Value string }
		}
		AllowedMentions struct{ Parse []string } `json:"allowed_mentions"`
	}
	if err := json.Unmarshal(payloads["/discord"].body, &discordBody); err != nil {
		t.Fatal(err)
	}
	if len(discordBody.Embeds) != 1 || discordBody.Embeds[0].Color == 0 || discordBody.Embeds[0].Description != "Vendor — Hosting" || len(discordBody.AllowedMentions.Parse) != 0 {
		t.Fatalf("missing Discord rich content: %s", payloads["/discord"].body)
	}
	if !strings.Contains(discordBody.Embeds[0].Fields[1].Value, "Assets:Checking  -15.2500 USD") || !strings.Contains(payloads["/discord"].query, "wait=true") || !strings.Contains(payloads["/discord"].query, "thread_id=123") {
		t.Fatal("Discord must include postings and preserve query parameters")
	}
	var slackBody struct {
		Text   string
		Blocks []struct {
			Type string
			Text struct{ Type, Text string }
		}
	}
	if err := json.Unmarshal(payloads["/slack"].body, &slackBody); err != nil {
		t.Fatal(err)
	}
	if slackBody.Text == "" || len(slackBody.Blocks) != 5 || slackBody.Blocks[0].Type != "header" || slackBody.Blocks[1].Text.Text != "Vendor — Hosting" || !strings.Contains(slackBody.Blocks[3].Text.Text, "15.2500 USD") {
		t.Fatalf("missing Slack rich content: %s", payloads["/slack"].body)
	}
}

func TestDeliveryRetryPolicy(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadRequest, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := calls.Add(1)
				w.Header().Set("Retry-After", "0")
				w.Header().Set("Location", "/redirected")
				if (status == 429 || status == 503) && count == 3 {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			n := New(ctx, Config{JSONURL: server.URL}, "Beanframe")
			err := n.deliver(n.destinations[0], Event{MutationEvent: mutation("transaction.created")})
			want := int32(1)
			if status == 429 || status == 503 {
				want = 3
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("permanent failures and redirects must be rejected")
			}
			if calls.Load() != want {
				t.Fatalf("got %d attempts, want %d", calls.Load(), want)
			}
		})
	}
}

func TestSlowDestinationDoesNotBlockWritesOrOtherDestinations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer slow.Close()
	defer close(release)
	delivered := make(chan struct{}, 1)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { delivered <- struct{}{} }))
	defer fast.Close()
	n := New(ctx, Config{DiscordURL: slow.URL, JSONURL: fast.URL}, "Beanframe")
	queued := make(chan struct{})
	go func() { n.Publish(mutation("transaction.created"), "revision"); close(queued) }()
	select {
	case <-queued:
	case <-time.After(time.Second):
		t.Fatal("notification blocked the write")
	}
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("slow Discord delivery blocked the JSON destination")
	}
}

func TestDisabledAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if New(ctx, Config{}, "Beanframe") != nil {
		t.Fatal("unset destinations must disable notifications")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		cancel()
	}))
	defer server.Close()
	n := New(ctx, Config{JSONURL: server.URL}, "Beanframe")
	done := make(chan error, 1)
	go func() { done <- n.deliver(n.destinations[0], Event{}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled delivery must fail")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the retry wait")
	}
}

func TestRichPayloadLimits(t *testing.T) {
	tx := transaction()
	tx.Payee = strings.Repeat("🫘", 4000)
	tx.Narration = strings.Repeat("long narration", 1000)
	for range 100 {
		tx.Postings = append(tx.Postings, tx.Postings[0])
	}
	e := Event{MutationEvent: engine.MutationEvent{Type: "transaction.created", Transaction: &tx}}
	discordJSON, err := json.Marshal(discord(e, strings.Repeat("Company", 100)))
	if err != nil {
		t.Fatal(err)
	}
	var discordBody struct {
		Embeds []struct {
			Description string
			Fields      []struct{ Value string }
			Footer      struct{ Text string }
		}
	}
	if err := json.Unmarshal(discordJSON, &discordBody); err != nil {
		t.Fatal(err)
	}
	for _, embed := range discordBody.Embeds {
		if len(embed.Description) > 4096 || !utf8.ValidString(embed.Description) || len(embed.Footer.Text) > 2048 {
			t.Fatal("Discord description or footer exceeds the provider limits")
		}
		for _, field := range embed.Fields {
			if len(field.Value) > 1024 || !utf8.ValidString(field.Value) {
				t.Fatal("Discord field exceeds its limit or splits Unicode")
			}
		}
	}
	slackJSON, err := json.Marshal(slack(e, "Beanframe"))
	if err != nil {
		t.Fatal(err)
	}
	var slackBody struct {
		Blocks []struct{ Text struct{ Text string } }
	}
	if err := json.Unmarshal(slackJSON, &slackBody); err != nil {
		t.Fatal(err)
	}
	for _, block := range slackBody.Blocks {
		if len(block.Text.Text) > 3000 || !utf8.ValidString(block.Text.Text) {
			t.Fatal("Slack section exceeds its limit or splits Unicode")
		}
	}
}

func TestEventSelection(t *testing.T) {
	for _, mode := range []string{"", "created", "transactions", "all"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			received := make(chan string, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var event Event
				if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
					t.Error(err)
				}
				received <- event.Type
			}))
			defer server.Close()
			n := New(ctx, Config{JSONURL: server.URL, Events: mode}, "Beanframe")
			kinds := []string{"transaction.updated", "transaction.deleted", "account.opened", "account.closed", "ledger.file_written", "ledger.restored", "transaction.created"}
			for _, kind := range kinds {
				n.Publish(engine.MutationEvent{Type: kind}, "revision")
			}
			want := []string{"transaction.created"}
			if mode == "transactions" {
				want = []string{"transaction.updated", "transaction.deleted", "transaction.created"}
			} else if mode == "all" {
				want = kinds
			}
			for _, kind := range want {
				select {
				case got := <-received:
					if got != kind {
						t.Fatalf("%s mode delivered %s, want %s", mode, got, kind)
					}
				case <-time.After(time.Second):
					t.Fatalf("%s mode did not deliver %s", mode, kind)
				}
			}
			// The final create follows every other event in this destination's queue.
			select {
			case got := <-received:
				t.Fatalf("unexpected event: %s", got)
			default:
			}
		})
	}
}

func TestNonTransactionRichMessages(t *testing.T) {
	for _, kind := range []string{"account.opened", "account.closed", "ledger.file_written", "ledger.restored"} {
		e := Event{MutationEvent: engine.MutationEvent{Type: kind, File: "main.beancount"}}
		if strings.HasPrefix(kind, "account.") {
			e.Account, e.Date = "Expenses:Hosting", "2026-04-01"
		}
		for _, payload := range []any{discord(e, "Beanframe"), slack(e, "Beanframe")} {
			body, err := json.Marshal(payload)
			if err != nil || !strings.Contains(string(body), title(e)) || !strings.Contains(string(body), "main.beancount") {
				t.Fatalf("incomplete %s rich message: %s %v", kind, body, err)
			}
			if e.Account != "" && (!strings.Contains(string(body), e.Account) || !strings.Contains(string(body), e.Date)) {
				t.Fatalf("account event must include account and date: %s", body)
			}
		}
	}
}
