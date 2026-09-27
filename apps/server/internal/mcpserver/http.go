package mcpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/prayujt/beanframe/apps/server/internal/ledger"
	"github.com/prayujt/beanframe/apps/server/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		SessionTimeout: 15 * time.Minute, JSONResponse: true, Logger: log.Slog(),
		MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true,
	})
	return http.NewCrossOriginProtection().Handler(handler)
}
