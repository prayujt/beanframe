package ledger

import (
	"context"
	"github.com/prayujt/beanframe/apps/server/internal/engine"
	"testing"
)

type stub struct{ data *engine.Snapshot }

func (s stub) Snapshot(context.Context) (*engine.Snapshot, error) { return s.data, nil }
func TestSearch(t *testing.T) {
	s := &Service{Engine: stub{&engine.Snapshot{Transactions: []engine.Transaction{
		{Date: "2026-01-01", Narration: "Hosting", Postings: []engine.Posting{{Account: "Expenses:Hosting"}}},
		{Date: "2026-01-02", Narration: "Hosting", Postings: []engine.Posting{{Account: "Expenses:Hosting"}}},
		{Date: "2026-01-03", Narration: "Hosting", Postings: []engine.Posting{{Account: "Expenses:HostingExtra"}}},
	}}}}
	page, err := s.Transactions(context.Background(), Filter{Account: "Expenses:Hosting", Text: "HOST", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Transactions) != 1 || page.NextOffset == nil || *page.NextOffset != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
	page, err = s.Transactions(context.Background(), Filter{Account: "Expenses", From: "2026-01-02", To: "2026-01-02"})
	if err != nil || page.Total != 1 {
		t.Fatalf("inclusive date filter: %+v %v", page, err)
	}
	for _, f := range []Filter{{From: "yesterday"}, {From: "2026-02-01", To: "2026-01-01"}, {Limit: 101}, {Offset: -1}} {
		if _, err := s.Transactions(context.Background(), f); err == nil {
			t.Errorf("accepted invalid filter %+v", f)
		}
	}
}
func TestValidationErrorsBlockReads(t *testing.T) {
	s := &Service{Engine: stub{&engine.Snapshot{Errors: []string{"unbalanced"}}}}
	if _, err := s.Load(context.Background()); err == nil {
		t.Fatal("accepted invalid ledger")
	}
	if result, err := s.Validate(context.Background()); err != nil || len(result.Errors) != 1 {
		t.Fatal("validation errors must remain queryable")
	}
}
