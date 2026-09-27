package rpcserver

import (
	"connectrpc.com/connect"
	"context"
	pb "github.com/prayujt/beanframe/apps/server/gen/beancount/v1"
	"github.com/prayujt/beanframe/apps/server/gen/beancount/v1/beancountv1connect"
	"github.com/prayujt/beanframe/apps/server/internal/auth"
	"github.com/prayujt/beanframe/apps/server/internal/config"
	"github.com/prayujt/beanframe/apps/server/internal/ledger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionAndWriteGuard(t *testing.T) {
	_, handler := Handler(&ledger.Service{}, config.Branding{Name: "Company", LogoURL: "https://example.com/logo.png"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r.WithContext(auth.WithAccess(r.Context(), auth.Access{Authenticated: true, AuthEnabled: true, Provider: "Test", User: auth.User{Subject: "reader", Email: "reader@example.com"}})))
	}))
	defer server.Close()
	c := beancountv1connect.NewLedgerServiceClient(server.Client(), server.URL)
	session, err := c.GetSession(context.Background(), connect.NewRequest(&pb.Empty{}))
	if err != nil || !session.Msg.Authenticated || session.Msg.CanWrite {
		t.Fatalf("session: %+v %v", session, err)
	}
	_, err = c.SaveTransaction(context.Background(), connect.NewRequest(&pb.SaveTransactionRequest{}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("read-only session write: %v", err)
	}
}

func TestPublicBrandingDoesNotReadLedger(t *testing.T) {
	_, handler := Handler(nil, config.Branding{CompanyName: "Example Company", Name: "Example Finance", LogoURL: "https://example.com/logo.png"})
	server := httptest.NewServer(handler)
	defer server.Close()
	c := beancountv1connect.NewLedgerServiceClient(server.Client(), server.URL)
	r, err := c.GetSession(context.Background(), connect.NewRequest(&pb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Msg.Authenticated || r.Msg.User != nil || r.Msg.CompanyName != "Example Company" || r.Msg.BrandName != "Example Finance" || r.Msg.BrandLogoUrl != "https://example.com/logo.png" {
		t.Fatalf("unexpected public session: %v", r.Msg)
	}
}
