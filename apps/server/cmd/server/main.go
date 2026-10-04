package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prayujt/beanframe/apps/server/internal/auth"
	"github.com/prayujt/beanframe/apps/server/internal/config"
	"github.com/prayujt/beanframe/apps/server/internal/engine"
	"github.com/prayujt/beanframe/apps/server/internal/ledger"
	"github.com/prayujt/beanframe/apps/server/internal/mcpserver"
	"github.com/prayujt/beanframe/apps/server/internal/notifications"
	"github.com/prayujt/beanframe/apps/server/internal/rpcserver"
	"github.com/prayujt/beanframe/apps/server/internal/web"
	"github.com/prayujt/beanframe/apps/server/pkg/log"
)

func run(ctx context.Context) error {
	if len(os.Args) > 1 {
		return fmt.Errorf("run the binary without subcommands; configure it with environment variables (see README)")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := log.Configure(log.Params{Level: cfg.LogLevel, Format: cfg.LogFormat}); err != nil {
		return err
	}
	if err := ledger.Seed(cfg.LedgerPath, cfg.LedgerSeedDir); err != nil {
		return err
	}
	client := &engine.Client{Python: cfg.Python, Path: cfg.LedgerPath}
	if _, err := client.Snapshot(ctx); err != nil {
		return err
	}
	service := &ledger.Service{Engine: client, Workspace: client}
	if notifier := notifications.New(ctx, cfg.Webhooks, cfg.Branding.Name); notifier != nil {
		service.OnMutation = notifier.Publish
	}
	go service.Watch(ctx)
	handler, err := web.Handler(cfg.WebDir, mcpserver.Handler(ctx, service))
	if err != nil {
		return err
	}
	security, err := auth.New(ctx, cfg.Auth)
	if err != nil {
		return err
	}
	routes := http.NewServeMux()
	rpcPath, rpcHandler := rpcserver.Handler(service, cfg.Branding)
	routes.Handle(rpcPath, rpcHandler)
	routes.Handle("/", handler)
	handler = security.Wrap(routes)
	server := &http.Server{
		Addr: cfg.ListenAddr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		ErrorLog: slog.NewLogLogger(log.Slog().Handler(), slog.LevelError),
	}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	log.Info("serving UI and MCP", "address", cfg.ListenAddr, "mcp_path", "/mcp", "authentication_disabled", cfg.Auth.Disabled)
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		log.Info("shutting down server")
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
