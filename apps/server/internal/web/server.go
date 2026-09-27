// Package web routes the shared HTTP listener to the UI and backend services.
package web

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/prayujt/beanframe/apps/server/pkg/log"
)

func Handler(dir string, mcpHandler http.Handler) (http.Handler, error) {
	fallback := filepath.Join(dir, "200.html")
	if info, err := os.Stat(fallback); err != nil || info.IsDir() {
		return nil, fmt.Errorf("build the SvelteKit app first: %s must be a file", fallback)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	// Reserve these paths so no ledger endpoint can be mistaken for a static page.
	unavailable := func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not implemented", http.StatusNotImplemented)
	}
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/mcp/", http.NotFound)
	mux.HandleFunc("/api", unavailable)
	mux.HandleFunc("/api/", unavailable)
	mux.HandleFunc("/auth", unavailable)
	mux.HandleFunc("/auth/", unavailable)
	mux.HandleFunc("/.well-known/", http.NotFound)
	files := http.FileServer(http.Dir(dir))
	// CodeMirror mounts styles at runtime. Give only those styles a per-response
	// nonce while preserving SvelteKit's script hashes and other CSP directives.
	serveHTML := func(w http.ResponseWriter, r *http.Request, filename string) {
		body, err := os.ReadFile(filename)
		if err != nil {
			http.Error(w, "Unable to load page", http.StatusInternalServerError)
			return
		}
		body = bytes.ReplaceAll(body, []byte("__LEDGER_STYLE_NONCE__"), []byte(rand.Text()))
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			if strings.HasSuffix(name, ".html") {
				serveHTML(w, r, filepath.Join(dir, name))
				return
			}
			files.ServeHTTP(w, r)
			return
		}
		// A missing asset must be a 404, not an HTML response. SvelteKit owns
		// navigation routes; Go provides its fallback for direct links/refreshes.
		if strings.HasPrefix(r.URL.Path, "/_app/") ||
			!strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.NotFound(w, r)
			return
		}
		serveHTML(w, r, fallback)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := rand.Text()
		ctx := log.WithAttrs(r.Context(), "request_id", requestID, "method", r.Method, "path", r.URL.Path)
		r = r.WithContext(ctx)
		started := time.Now()
		defer func() { log.DebugCtx(ctx, "HTTP request completed", "duration", time.Since(started)) }()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// SvelteKit emits a CSP meta tag with hashes for its inline bootstrap.
		// frame-ancestors must be an HTTP header and is additive to that policy.
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'self'")
		mux.ServeHTTP(w, r)
	}), nil
}
