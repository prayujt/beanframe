package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/prayujt/beanframe/apps/server/gen/beancount/v1"
	"github.com/prayujt/beanframe/apps/server/gen/beancount/v1/beancountv1connect"
	"github.com/prayujt/beanframe/apps/server/internal/notifications"
)

// This exercises the HTTP protocol, compiled server, UI, and Python bridge.
func TestHTTPIntegration(t *testing.T) {
	python := os.Getenv("TEST_PYTHON")
	if python == "" {
		t.Skip("set TEST_PYTHON to run the full Python/MCP integration test (make test does this)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "server")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/server")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	fixture, err := filepath.Abs("../../../../testdata/ledger")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	err = filepath.WalkDir(fixture, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(fixture, path)
		target := filepath.Join(root, relative)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(root, "main.beancount")
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "200.html"), []byte("<html>test UI</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	endpoint := "http://" + address
	webhooks := make(chan notifications.Event, 8)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event notifications.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Error(err)
		}
		webhooks <- event
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "AUTH_DISABLED=true", "LISTEN_ADDR="+address, "WEB_DIR="+webDir, "LEDGER_PATH="+ledgerPath, "PYTHON="+python, "DISCORD_WEBHOOK_URL=", "SLACK_WEBHOOK_URL=", "JSON_WEBHOOK_URL="+webhook.URL, "WEBHOOK_EVENTS=created")
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if err := cmd.Wait(); err != nil {
			t.Errorf("server shutdown: %v: %s", err, logs.String())
		}
	}()
	httpClient := &http.Client{Timeout: time.Second}
	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		response, err := httpClient.Get(endpoint + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become ready")
	}
	t.Run("stateless discovery and tool call", func(t *testing.T) {
		client := mcp.NewClient(&mcp.Implementation{Name: "stateless-integration", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint + "/mcp"}, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		if version := session.InitializeResult().ProtocolVersion; version != "2026-07-28" {
			t.Fatalf("discovery fell back to %s", version)
		}
		list, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(list.Tools) != 14 {
			t.Fatalf("got %d tools", len(list.Tools))
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_balances", Arguments: map[string]any{"account": "Assets:Checking"}})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("get_balances: %+v", result)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil || !strings.Contains(string(raw), "117.66") || !strings.Contains(string(raw), "EUR") {
			t.Fatalf("get_balances: %s %v", raw, err)
		}
	})
	notified := make(chan struct{}, 10)
	client := mcp.NewClient(&mcp.Implementation{Name: "integration", Version: "1"}, &mcp.ClientOptions{ResourceUpdatedHandler: func(_ context.Context, r *mcp.ResourceUpdatedNotificationRequest) {
		if r.Params.URI == "ledger://snapshot" {
			select {
			case notified <- struct{}{}:
			default:
			}
		}
	}})
	// Exercise the legacy session's subscriptions alongside modern tool discovery.
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint + "/mcp"}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	request, _ := http.NewRequestWithContext(ctx, "GET", endpoint+"/preview/reader", nil)
	request.Header.Set("Accept", "text/html")
	response, err := httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), "test UI") {
		t.Fatalf("UI unavailable alongside MCP: %d %s %v", response.StatusCode, body, err)
	}
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 14 {
		t.Fatalf("got %d tools", len(list.Tools))
	}
	for _, tool := range list.Tools {
		if tool.Annotations == nil {
			t.Errorf("tool %s missing annotations", tool.Name)
		}
	}
	call := func(name string, args map[string]any) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v %v", name, result, err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	if out := call("get_balances", map[string]any{"account": "Assets:Checking"}); !strings.Contains(out, "117.66") || !strings.Contains(out, "EUR") {
		t.Fatal(out)
	}
	if out := call("list_accounts", map[string]any{}); !strings.Contains(out, "Liabilities:Founder") {
		t.Fatal(out)
	}
	if out := call("list_transactions", map[string]any{"account": "Expenses", "limit": 1}); !strings.Contains(out, `"total":2`) || !strings.Contains(out, `"next_offset":1`) {
		t.Fatal(out)
	}
	if out := call("validate_ledger", map[string]any{}); !strings.Contains(out, `"valid":true`) {
		t.Fatal(out)
	}
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_transactions", Arguments: map[string]any{"limit": 101}})
	if err != nil || !bad.IsError {
		t.Fatalf("invalid filter must be a tool error: %+v %v", bad, err)
	}

	rpcClient := beancountv1connect.NewLedgerServiceClient(http.DefaultClient, endpoint)
	initial, err := rpcClient.GetSnapshot(ctx, connect.NewRequest(&pb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}
	first, err := rpcClient.WatchLedger(ctx, connect.NewRequest(&pb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := rpcClient.WatchLedger(ctx, connect.NewRequest(&pb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if !first.Receive() || !second.Receive() {
		t.Fatal("missing initial stream revision")
	}
	if err = session.Subscribe(ctx, &mcp.SubscribeParams{URI: "ledger://snapshot"}); err != nil {
		t.Fatal(err)
	}
	source := "2026-02-10 * \"Live MCP edit\"\n  Expenses:Hosting  5 USD\n  Assets:Checking -5 USD\n"
	call("save_transaction", map[string]any{"source": source, "expected_revision": initial.Msg.Revision})
	assertWebhook := func(t *testing.T, narration, revision string) {
		t.Helper()
		select {
		case event := <-webhooks:
			if event.Type != "transaction.created" || event.Transaction == nil || event.Transaction.Narration != narration || event.Revision != revision || len(event.Transaction.Postings) != 2 {
				t.Fatalf("incorrect transaction webhook: %+v", event)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("transaction create did not deliver a webhook")
		}
	}
	for _, stream := range []*connect.ServerStreamForClient[pb.LedgerEvent]{first, second} {
		for stream.Receive() {
			if stream.Msg().Revision != "" && stream.Msg().Revision != initial.Msg.Revision {
				break
			}
		}
		if stream.Err() != nil {
			t.Fatal(stream.Err())
		}
	}
	select {
	case <-notified:
	case <-time.After(5 * time.Second):
		t.Fatal("MCP resource subscriber did not receive update")
	}
	updated, err := rpcClient.GetSnapshot(ctx, connect.NewRequest(&pb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}
	assertWebhook(t, "Live MCP edit", updated.Msg.Revision)
	var txID string
	for _, tx := range updated.Msg.Transactions {
		if tx.Narration == "Live MCP edit" {
			txID = tx.Id
		}
	}
	if txID == "" {
		t.Fatal("Connect did not see MCP mutation")
	}
	stale, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "save_transaction", Arguments: map[string]any{"source": source, "expected_revision": initial.Msg.Revision}})
	if err != nil || !stale.IsError {
		t.Fatal("stale MCP mutation accepted")
	}
	_, err = rpcClient.DeleteTransaction(ctx, connect.NewRequest(&pb.DeleteTransactionRequest{Id: txID, ExpectedRevision: updated.Msg.Revision}))
	if err != nil {
		t.Fatal(err)
	}
	if out := call("list_transactions", map[string]any{"text": "Live MCP edit"}); !strings.Contains(out, `"total":0`) {
		t.Fatal(out)
	}
	first.Close()
	second.Close()
	// External edits trigger subscriptions and are visible to both transports.
	external, err := rpcClient.WatchLedger(ctx, connect.NewRequest(&pb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	if !external.Receive() {
		t.Fatal("no external watcher initial event")
	}
	externalRevision := external.Msg().Revision
	path := filepath.Join(root, "transactions", "2026.beancount")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("\n2026-03-01 * \"Another loan\"\n  Assets:Checking  1.00 USD\n  Liabilities:Founder\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	for external.Receive() {
		if external.Msg().Revision != "" && external.Msg().Revision != externalRevision {
			break
		}
	}
	if external.Err() != nil {
		t.Fatal(external.Err())
	}
	if out := call("get_balances", map[string]any{"account": "Assets:Checking"}); !strings.Contains(out, "118.66") {
		t.Fatal(out)
	}
	t.Run("UI transaction webhooks", func(t *testing.T) {
		snapshot, err := rpcClient.GetSnapshot(ctx, connect.NewRequest(&pb.Empty{}))
		if err != nil {
			t.Fatal(err)
		}
		source := "2026-04-01 * \"UI webhook\"\n  Expenses:Hosting  15.25 USD\n  Assets:Checking\n"
		created, err := rpcClient.SaveTransaction(ctx, connect.NewRequest(&pb.SaveTransactionRequest{Source: source, File: "transactions/2026.beancount", ExpectedRevision: snapshot.Msg.Revision}))
		if err != nil {
			t.Fatal(err)
		}
		assertWebhook(t, "UI webhook", created.Msg.Revision)
		snapshot, err = rpcClient.GetSnapshot(ctx, connect.NewRequest(&pb.Empty{}))
		if err != nil {
			t.Fatal(err)
		}
		var id string
		for _, tx := range snapshot.Msg.Transactions {
			if tx.Narration == "UI webhook" {
				id = tx.Id
			}
		}
		edited, err := rpcClient.SaveTransaction(ctx, connect.NewRequest(&pb.SaveTransactionRequest{Id: id, Source: strings.ReplaceAll(source, "15.25", "16.25"), ExpectedRevision: created.Msg.Revision}))
		if err != nil {
			t.Fatal(err)
		}
		_, err = rpcClient.SaveTransaction(ctx, connect.NewRequest(&pb.SaveTransactionRequest{Source: "2026-04-01 * \"Invalid\"\n  Expenses:Hosting  1 USD\n", ExpectedRevision: edited.Msg.Revision}))
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("invalid transaction must fail: %v", err)
		}
	})
	select {
	case event := <-webhooks:
		t.Fatalf("startup, edits, deletes, failed writes, and external edits must not notify: %+v", event)
	case <-time.After(250 * time.Millisecond):
	}
}
