package notifications

import (
	"strings"
	"unicode/utf8"

	"github.com/prayujt/beanframe/apps/server/internal/engine"
)

func summary(tx engine.Transaction) string {
	parts := []string{}
	for _, value := range []string{tx.Payee, tx.Narration} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return "New transaction"
	}
	return strings.Join(parts, " — ")
}

func postings(tx engine.Transaction) string {
	lines := make([]string, 0, len(tx.Postings))
	for _, p := range tx.Postings {
		line := p.Account
		if p.Units != nil {
			line += "  " + p.Units.Number + " " + p.Units.Currency
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "No postings"
	}
	return strings.Join(lines, "\n")
}

// A UTF-8 byte budget conservatively fits the providers' character limits.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit - len("…")
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "…"
}

func title(e Event) string {
	return map[string]string{
		"transaction.created": "Transaction created", "transaction.updated": "Transaction updated",
		"transaction.deleted": "Transaction deleted", "account.opened": "Account opened",
		"account.closed": "Account closed", "ledger.file_written": "Ledger file saved",
		"ledger.restored": "Ledger version restored",
	}[e.Type]
}

func details(e Event) (string, []any) {
	if e.Transaction != nil {
		return summary(*e.Transaction), []any{
			map[string]any{"name": "Date", "value": e.Transaction.Date, "inline": true},
			map[string]any{"name": "Postings", "value": truncate(postings(*e.Transaction), 1024)},
		}
	}
	fields := []any{map[string]any{"name": "File", "value": truncate(e.File, 1024)}}
	if e.Date != "" {
		fields = append(fields, map[string]any{"name": "Date", "value": e.Date, "inline": true})
	}
	if e.Account != "" {
		return e.Account, fields
	}
	return e.File, fields
}

func discord(e Event, name string) any {
	description, fields := details(e)
	return map[string]any{
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []any{map[string]any{
			"title": title(e), "description": truncate(description, 1024),
			"color": 0x3498db, "timestamp": e.OccurredAt,
			"fields": fields,
			"footer": map[string]any{"text": truncate(name, 256)},
		}},
	}
}

func slack(e Event, name string) any {
	text := func(value string) any { return map[string]any{"type": "plain_text", "text": value} }
	description, _ := details(e)
	blocks := []any{
		map[string]any{"type": "header", "text": text(title(e))},
		map[string]any{"type": "section", "text": text(truncate(description, 3000))},
	}
	if e.Transaction != nil {
		blocks = append(blocks,
			map[string]any{"type": "section", "fields": []any{text("Date\n" + e.Transaction.Date)}},
			map[string]any{"type": "section", "text": text(truncate(postings(*e.Transaction), 3000))},
		)
	} else {
		blocks = append(blocks, map[string]any{"type": "section", "text": text(truncate("File: "+e.File, 3000))})
		if e.Date != "" {
			blocks = append(blocks, map[string]any{"type": "section", "text": text("Date: " + e.Date)})
		}
	}
	blocks = append(blocks, map[string]any{"type": "context", "elements": []any{text(truncate(name, 256))}})
	return map[string]any{
		"text": title(e), "blocks": blocks,
	}
}
