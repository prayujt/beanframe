package mcpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prayujt/beanframe/apps/server/internal/ledger"
	"github.com/prayujt/beanframe/apps/server/pkg/log"
)

// Handler serves tools and resource subscriptions over Streamable HTTP.
func Handler(ctx context.Context, service *ledger.Service) http.Handler {
	server := New(service)
	changes, unsubscribe := service.Subscribe()
	go func() {
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case <-changes:
				_ = server.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: "ledger://snapshot"})
			}
		}
	}()
	getServer := func(*http.Request) *mcp.Server { return server }
	opts := mcp.StreamableHTTPOptions{
		SessionTimeout: 15 * time.Minute, JSONResponse: true, Logger: log.Slog(),
		MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true,
	}
	stateful := mcp.NewStreamableHTTPHandler(getServer, &opts)
	opts.Stateless = true
	stateless := mcp.NewStreamableHTTPHandler(getServer, &opts)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The SDK requires stateless HTTP for protocol 2026-07-28 onward.
		// Keep legacy sessions and their resource notification streams intact.
		if r.Header.Get("Mcp-Protocol-Version") >= "2026-07-28" {
			stateless.ServeHTTP(w, r)
			return
		}
		stateful.ServeHTTP(w, r)
	})
	return http.NewCrossOriginProtection().Handler(handler)
}
