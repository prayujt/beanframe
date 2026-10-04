// Package ledger owns accounting operations independently of MCP or HTTP.
package ledger

import (
	"context"
	"fmt"
	"github.com/prayujt/beanframe/apps/server/internal/engine"
	"strings"
	"sync"
	"time"
)

type Reader interface {
	Snapshot(context.Context) (*engine.Snapshot, error)
}
type Service struct {
	Engine      Reader
	Workspace   *engine.Client
	OnMutation  func(engine.MutationEvent, string)
	mu          sync.RWMutex
	eventMu     sync.Mutex
	subscribers map[chan Event]struct{}
	revision    string
}

func (s *Service) Validate(ctx context.Context) (*engine.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Engine.Snapshot(ctx)
}
func (s *Service) Load(ctx context.Context) (*engine.Snapshot, error) {
	snapshot, err := s.Validate(ctx)
	if err != nil {
		return nil, err
	}
	if len(snapshot.Errors) > 0 {
		return nil, fmt.Errorf("ledger has %d validation errors; use validate_ledger", len(snapshot.Errors))
	}
	return snapshot, nil
}

type Filter struct {
	From    string `json:"from,omitempty" jsonschema:"Inclusive start date YYYY-MM-DD"`
	To      string `json:"to,omitempty" jsonschema:"Inclusive end date YYYY-MM-DD"`
	Account string `json:"account,omitempty" jsonschema:"Exact account or parent account including descendants"`
	Text    string `json:"text,omitempty" jsonschema:"Case-insensitive payee or narration search"`
	Offset  int    `json:"offset,omitempty" jsonschema:"Zero-based offset"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 100; defaults to 50"`
}
type Page struct {
	Transactions []engine.Transaction `json:"transactions"`
	Total        int                  `json:"total"`
	NextOffset   *int                 `json:"next_offset"`
}

func MatchesAccount(name, prefix string) bool {
	return prefix == "" || name == prefix || strings.HasPrefix(name, prefix+":")
}
func (s *Service) Transactions(ctx context.Context, f Filter) (*Page, error) {
	for _, date := range []string{f.From, f.To} {
		if date != "" {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return nil, fmt.Errorf("invalid date %q; use YYYY-MM-DD", date)
			}
		}
	}
	if f.From != "" && f.To != "" && f.From > f.To {
		return nil, fmt.Errorf("from must not be after to")
	}
	if f.Limit == 0 {
		f.Limit = 50
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		return nil, fmt.Errorf("limit must be 1..100 and offset must be nonnegative")
	}
	snapshot, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}
	page := &Page{Transactions: []engine.Transaction{}}
	for _, tx := range snapshot.Transactions {
		if (f.From != "" && tx.Date < f.From) || (f.To != "" && tx.Date > f.To) {
			continue
		}
		if !strings.Contains(strings.ToLower(tx.Payee+" "+tx.Narration), strings.ToLower(f.Text)) {
			continue
		}
		matched := false
		for _, p := range tx.Postings {
			if MatchesAccount(p.Account, f.Account) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if page.Total >= f.Offset && len(page.Transactions) < f.Limit {
			page.Transactions = append(page.Transactions, tx)
		}
		page.Total++
	}
	if f.Offset < page.Total && len(page.Transactions) < page.Total-f.Offset {
		n := f.Offset + len(page.Transactions)
		page.NextOffset = &n
	}
	return page, nil
}
