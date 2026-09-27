package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prayujt/beanframe/apps/server/internal/engine"
	"github.com/prayujt/beanframe/apps/server/internal/ledger"
	"github.com/prayujt/beanframe/apps/server/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type empty struct{}
type accountInput struct {
	Account string `json:"account,omitempty" jsonschema:"Exact account or parent account including descendants"`
}
type validation struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}
type accounts struct {
	Accounts []engine.Account `json:"accounts"`
}
type balances struct {
	Balances []engine.Balance `json:"balances"`
}

func New(service *ledger.Service) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "beanframe", Version: "0.2.0"}, &mcp.ServerOptions{Logger: log.Slog(),
		SubscribeHandler: func(_ context.Context, r *mcp.SubscribeRequest) error {
			if r.Params.URI != "ledger://snapshot" {
				return fmt.Errorf("unknown resource")
			}
			return nil
		},
		UnsubscribeHandler: func(_ context.Context, _ *mcp.UnsubscribeRequest) error { return nil },
	})
	tool := func(name, description string) *mcp.Tool {
		return &mcp.Tool{Name: name, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}
	}
	mcp.AddTool(s, tool("validate_ledger", "Check the configured ledger and all included files for Beancount errors."), func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, validation, error) {
		snapshot, err := service.Validate(ctx)
		if err != nil {
			return nil, validation{}, err
		}
		return nil, validation{len(snapshot.Errors) == 0, snapshot.Errors}, nil
	})
	mcp.AddTool(s, tool("list_accounts", "List declared accounts and their opening dates and currency restrictions."), func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, accounts, error) {
		snapshot, err := service.Load(ctx)
		if err != nil {
			return nil, accounts{}, err
		}
		return nil, accounts{snapshot.Accounts}, nil
	})
	mcp.AddTool(s, tool("get_balances", "Get current account positions, preserving currencies and cost lots. No currency conversion or market valuation is performed."), func(ctx context.Context, _ *mcp.CallToolRequest, in accountInput) (*mcp.CallToolResult, balances, error) {
		snapshot, err := service.Load(ctx)
		if err != nil {
			return nil, balances{}, err
		}
		out := balances{Balances: []engine.Balance{}}
		for _, b := range snapshot.Balances {
			if ledger.MatchesAccount(b.Account, in.Account) {
				out.Balances = append(out.Balances, b)
			}
		}
		return nil, out, nil
	})
	mcp.AddTool(s, tool("list_transactions", "Search transactions by inclusive dates, account subtree, and payee/narration. Results are chronological and paginated."), func(ctx context.Context, _ *mcp.CallToolRequest, in ledger.Filter) (*mcp.CallToolResult, *ledger.Page, error) {
		page, err := service.Transactions(ctx, in)
		return nil, page, err
	})
	s.AddResource(&mcp.Resource{URI: "ledger://snapshot", Name: "Ledger snapshot", Description: "Current ledger and revision; subscribe for change notifications", MIMEType: "application/json"}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		value, err := service.Validate(ctx)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "ledger://snapshot", MIMEType: "application/json", Text: string(raw)}}}, nil
	})
	addWorkspaceTools(s, service)
	return s
}
