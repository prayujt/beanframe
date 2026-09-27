package auth

import (
	"context"
	"net/http"
	"time"
)

const RPCPrefix = "/beancount.v1.LedgerService/"
const WriteScope = "beancount:write"

type accessKey struct{}
type Access struct {
	Authenticated bool
	AuthEnabled   bool
	Provider      string
	User          User
	Expires       time.Time
	CanWrite      bool
	Actor         string
	Renewable     bool
	valid         func() bool
}

func FromContext(ctx context.Context) Access { a, _ := ctx.Value(accessKey{}).(Access); return a }
func (a Access) Valid() bool                 { return a.valid == nil || a.valid() }
func WithAccess(ctx context.Context, access Access) context.Context {
	return context.WithValue(ctx, accessKey{}, access)
}
func (a *Auth) browserAccess(r *http.Request) Access {
	if a.cfg.Disabled {
		return Access{AuthEnabled: false, CanWrite: true, Actor: "development"}
	}
	s, ok := a.current(r)
	result := Access{Authenticated: ok, AuthEnabled: true, Provider: a.cfg.ProviderName, User: s.User, Expires: s.Expires, CanWrite: ok, Actor: "ui:" + s.User.Subject, Renewable: s.renewable()}
	result.valid = func() bool { _, ok := a.current(r); return ok }
	return result
}
func (a *Auth) rpc(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Use POST", 405)
			return
		}
		if !a.cfg.Disabled && r.Header.Get("Origin") != a.cfg.PublicURL {
			jsonResponse(w, 403, map[string]string{"code": "permission_denied", "message": "Invalid origin"})
			return
		}
		access := a.browserAccess(r)
		if r.URL.Path != RPCPrefix+"GetSession" && access.AuthEnabled && !access.Authenticated {
			jsonResponse(w, 401, map[string]string{"code": "unauthenticated", "message": "Sign in to continue"})
			return
		}
		if access.Authenticated && access.Renewable {
			if c, err := r.Cookie(a.cookieName("session")); err == nil {
				a.cookie(w, "session", c.Value, int(sessionRetention.Seconds()))
			}
		}
		next.ServeHTTP(w, r.WithContext(WithAccess(r.Context(), access)))
	}
}
