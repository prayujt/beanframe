package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/prayujt/beanframe/apps/server/internal/auth"
	"github.com/prayujt/beanframe/apps/server/internal/ledger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type reportInput struct {
	From     string `json:"from,omitempty" jsonschema:"Inclusive start date YYYY-MM-DD"`
	To       string `json:"to,omitempty" jsonschema:"Inclusive end date YYYY-MM-DD"`
	Currency string `json:"currency,omitempty" jsonschema:"Report currency; currencies are never combined or converted"`
}
type fileInput struct {
	Path string `json:"path" jsonschema:"Relative file path returned by get_ledger_snapshot"`
}
type saveInput struct {
	ExpectedRevision string `json:"expected_revision" jsonschema:"Exact revision from a fresh get_ledger_snapshot; stale edits are rejected"`
	ID               string `json:"id,omitempty" jsonschema:"Existing transaction ID to edit; omit to create a transaction"`
	File             string `json:"file,omitempty" jsonschema:"Existing relative ledger file for new transactions; defaults to root ledger"`
	Source           string `json:"source" jsonschema:"Exactly one complete Beancount transaction, with date, flag, narration and balanced postings"`
}
type deleteInput struct {
	ExpectedRevision string `json:"expected_revision"`
	ID               string `json:"id" jsonschema:"Transaction ID from get_ledger_snapshot or list_transactions"`
}
type openInput struct {
	ExpectedRevision string   `json:"expected_revision"`
	Name             string   `json:"name"`
	Date             string   `json:"date"`
	Currencies       []string `json:"currencies"`
}
type closeInput struct {
	ExpectedRevision string `json:"expected_revision"`
	Name             string `json:"name"`
	Date             string `json:"date"`
}
type writeInput struct {
	ExpectedRevision string `json:"expected_revision"`
	Path             string `json:"path"`
	Content          string `json:"content"`
}
type restoreInput struct {
	ExpectedRevision string `json:"expected_revision"`
	ID               string `json:"id" jsonschema:"History entry whose prior file content should be restored"`
}

func queryTool[T any](s *mcp.Server, service *ledger.Service, name, op, description string) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, args T) (*mcp.CallToolResult, map[string]any, error) {
		raw, err := service.Query(ctx, op, args)
		if err != nil {
			return nil, nil, err
		}
		var out map[string]any
		err = json.Unmarshal(raw, &out)
		return nil, out, err
	})
}
func writeTool[T any](s *mcp.Server, service *ledger.Service, name, op, description string) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false}}, func(ctx context.Context, _ *mcp.CallToolRequest, in T) (*mcp.CallToolResult, *ledger.MutationResult, error) {
		access := auth.FromContext(ctx)
		if !access.CanWrite {
			return nil, nil, fmt.Errorf("beancount:write scope is required; sign in with write access")
		}
		raw, err := json.Marshal(in)
		if err != nil {
			return nil, nil, err
		}
		var args ledger.Mutation
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, nil, err
		}
		args.Actor = access.Actor
		out, err := service.Mutate(ctx, op, args)
		return nil, out, err
	})
}
func addWorkspaceTools(s *mcp.Server, service *ledger.Service) {
	queryTool[empty](s, service, "get_ledger_snapshot", "snapshot", "Read the ledger, file list, validation diagnostics and revision. Fetch before every write; amounts are exact decimal strings.")
	queryTool[reportInput](s, service, "get_report", "report", "Income statement, balance sheet, trial balance and monthly trends at book cost in one currency. No FX conversion or market valuation.")
	queryTool[fileInput](s, service, "read_ledger_file", "read_file", "Read an existing ledger file and its workspace revision.")
	queryTool[empty](s, service, "get_ledger_history", "history", "List up to 200 recovery records, newest first, including actor and changed file.")
	writeTool[saveInput](s, service, "save_transaction", "save_transaction", "Create or edit one transaction. Requires beancount:write and current expected_revision. Validates the complete ledger, saves atomically, records recovery history and broadcasts the change.")
	writeTool[deleteInput](s, service, "delete_transaction", "delete_transaction", "Delete one transaction after validating the resulting ledger; recoverable through history. Requires current revision and write scope.")
	writeTool[openInput](s, service, "open_account", "open_account", "Open an account with explicit currency restrictions. Requires current revision and write scope.")
	writeTool[closeInput](s, service, "close_account", "close_account", "Close a zero-balance account on the given date. Requires current revision and write scope.")
	writeTool[writeInput](s, service, "write_ledger_file", "write_file", "Replace an existing ledger file with validated content. Include/plugin configuration cannot be changed. Requires current revision and write scope.")
	writeTool[restoreInput](s, service, "restore_ledger_version", "restore_version", "Restore one file's content from before a recorded change. Validates against the current ledger and creates a new history record. Requires current revision and write scope.")
}
