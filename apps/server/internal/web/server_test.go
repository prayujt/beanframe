package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSPARouting(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "_app", "immutable"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"200.html": "<html>SPA shell</html>", "_app/immutable/app.js": "console.log('asset')"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h, err := Handler(dir, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) }))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		method, path, accept string
		status               int
		body                 string
	}{
		{"GET", "/", "text/html", 200, "SPA shell"},
		{"GET", "/preview/reader", "text/html", 200, "SPA shell"},
		{"GET", "/preview/a.b?view=example", "text/html", 200, "SPA shell"},
		{"HEAD", "/preview/reader", "text/html", 200, ""},
		{"GET", "/_app/immutable/app.js", "*/*", 200, "console.log"},
		{"GET", "/_app/immutable/missing.js", "text/html", 404, ""},
		{"GET", "/missing.js", "*/*", 404, ""},
		{"GET", "/preview/reader", "application/json", 404, ""},
		{"GET", "/healthz", "*/*", 200, `"status":"ok"`},
		{"POST", "/preview/reader", "text/html", 405, ""},
		{"GET", "/mcp", "text/html", 202, ""},
		{"POST", "/mcp", "application/json", 202, ""},
		{"GET", "/mcp/extra", "text/html", 404, ""},
		{"GET", "/api", "text/html", 501, ""},
		{"GET", "/api/balances", "text/html", 501, ""},
		{"GET", "/auth/login", "text/html", 501, ""},
		{"GET", "/.well-known/oauth-protected-resource", "text/html", 404, ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path+" "+tc.accept, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("got %d %q", w.Code, w.Body.String())
			}
			if tc.method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
			if tc.status != 200 && strings.Contains(w.Body.String(), "SPA shell") {
				t.Fatal("fallback swallowed a non-UI route")
			}
			if strings.Contains(tc.body, "SPA shell") && w.Header().Get("Cache-Control") != "no-cache" {
				t.Fatal("HTML fallback must revalidate")
			}
		})
	}
}

func TestMissingBuild(t *testing.T) {
	if _, err := Handler(t.TempDir(), http.NotFoundHandler()); err == nil {
		t.Fatal("expected missing build error")
	}
}

func TestStyleNonce(t *testing.T) {
	dir := t.TempDir()
	const shell = `<meta http-equiv="Content-Security-Policy" content="script-src 'self' 'sha256-script'; style-src 'self' 'nonce-__LEDGER_STYLE_NONCE__'"><meta name="style-nonce" content="__LEDGER_STYLE_NONCE__">`
	if err := os.WriteFile(filepath.Join(dir, "200.html"), []byte(shell), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := Handler(dir, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, path := range []string{"/files", "/files", "/200.html"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		body := w.Body.String()
		match := regexp.MustCompile(`name="style-nonce" content="([a-zA-Z0-9]+)"`).FindStringSubmatch(body)
		if w.Code != 200 || len(match) != 2 || len(match[1]) < 26 || seen[match[1]] {
			t.Fatalf("missing or reused nonce: %q", body)
		}
		seen[match[1]] = true
		if strings.Contains(body, "__LEDGER_STYLE_NONCE__") || !strings.Contains(body, "'nonce-"+match[1]+"'") || !strings.Contains(body, "script-src 'self' 'sha256-script'") {
			t.Fatalf("nonce and CSP do not match: %q", body)
		}
		if w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal("nonce-bearing HTML must revalidate")
		}
	}
}
