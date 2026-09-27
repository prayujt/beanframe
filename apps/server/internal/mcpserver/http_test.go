package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prayujt/beanframe/apps/server/internal/ledger"
)

func TestHTTPBoundary(t *testing.T) {
	h := Handler(t.Context(), &ledger.Service{})
	for _, tc := range []struct {
		name, method, origin, body string
		status                     int
	}{
		{"GET without session", "GET", "", "", http.StatusBadRequest},
		{"foreign origin", "POST", "https://foreign.example", `{}`, http.StatusForbidden},
		{"large body", "POST", "", strings.Repeat(" ", (1<<20)+1), http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://localhost/mcp", strings.NewReader(tc.body))
			r.Header.Set("Accept", "application/json, text/event-stream")
			r.Header.Set("Content-Type", "application/json")
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
