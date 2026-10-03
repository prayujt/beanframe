package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prayujt/beanframe/apps/server/internal/ledger"
)

func TestHTTPBoundary(t *testing.T) {
	h := Handler(t.Context(), &ledger.Service{})
	for _, tc := range []struct {
		name, method, origin, body, protocol string
		status                               int
	}{
		{"GET without session", "GET", "", "", "", http.StatusBadRequest},
		{"foreign origin", "POST", "https://foreign.example", `{}`, "", http.StatusForbidden},
		{"large body", "POST", "", strings.Repeat(" ", (1<<20)+1), "", http.StatusRequestEntityTooLarge},
		{"stateless GET", "GET", "", "", "2026-07-28", http.StatusMethodNotAllowed},
		{"stateless foreign origin", "POST", "https://foreign.example", `{}`, "2026-07-28", http.StatusForbidden},
		{"stateless large body", "POST", "", strings.Repeat(" ", (1<<20)+1), "2026-07-28", http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://localhost/mcp", strings.NewReader(tc.body))
			r.Header.Set("Accept", "application/json, text/event-stream")
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Mcp-Protocol-Version", tc.protocol)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// Discovery must advertise the new protocol, and tools/list must work without
// an initialize handshake or a session ID on a separate HTTP request.
func TestHTTPStatelessDiscovery(t *testing.T) {
	h := Handler(t.Context(), &ledger.Service{})
	request := func(method string, result any) {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": method,
			"params": map[string]any{"_meta": map[string]any{
				mcp.MetaKeyProtocolVersion:    "2026-07-28",
				mcp.MetaKeyClientInfo:         map[string]string{"name": "discovery-test", "version": "1"},
				mcp.MetaKeyClientCapabilities: map[string]any{},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(string(body)))
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Mcp-Protocol-Version", "2026-07-28")
		r.Header.Set("Mcp-Method", method)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", method, w.Code, w.Body.String())
		}
		if id := w.Header().Get("Mcp-Session-Id"); id != "" {
			t.Fatalf("%s returned a session ID: %q", method, id)
		}
		var response struct {
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Error) != 0 {
			t.Fatalf("%s: %s", method, response.Error)
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			t.Fatal(err)
		}
	}
	var discovery mcp.DiscoverResult
	request("server/discover", &discovery)
	if !slices.Contains(discovery.SupportedVersions, "2026-07-28") {
		t.Fatalf("new protocol missing from discovery: %v", discovery.SupportedVersions)
	}
	if discovery.Capabilities == nil || discovery.Capabilities.Tools == nil {
		t.Fatalf("tool capability missing: %+v", discovery.Capabilities)
	}
	var tools mcp.ListToolsResult
	request("tools/list", &tools)
	if len(tools.Tools) != 14 {
		t.Fatalf("expected 14 tools, got %d", len(tools.Tools))
	}
}
