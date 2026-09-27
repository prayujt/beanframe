package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const mcpScope = "beancount:read"
const pendingTTL = 10 * time.Minute
const maxEntries = 4096

type User struct {
	Subject  string   `json:"sub"`
	Name     string   `json:"name"`
	Username string   `json:"preferred_username"`
	Email    string   `json:"email"`
	Groups   []string `json:"groups,omitempty"`
}
type session struct {
	User                    User
	Expires                 time.Time
	Token                   *oauth2.Token
	SID, Nonce              string
	RefreshMu               *sync.Mutex
	RetainUntil, CheckAfter time.Time
}
type pending struct {
	State, Nonce, Verifier string
	Expires                time.Time
}

type Auth struct {
	cfg                   Config
	provider, mcpProvider *oidc.Provider
	verifier, mcpVerifier *oidc.IDTokenVerifier
	oauth                 oauth2.Config
	client                *http.Client
	mu                    sync.Mutex
	sessions              map[[32]byte]session
	pending               map[[32]byte]pending
	logoutSeen            map[string]time.Time
	userInfoURL           string
}

func New(ctx context.Context, cfg Config) (*Auth, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &Auth{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}, sessions: make(map[[32]byte]session), pending: make(map[[32]byte]pending), logoutSeen: make(map[string]time.Time)}
	if cfg.Disabled {
		return a, nil
	}
	// Verifiers retain their HTTP client for later JWKS fetches, not a startup deadline.
	discoveryCtx := oidc.ClientContext(ctx, a.client)
	var err error
	a.provider, err = oidc.NewProvider(discoveryCtx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	a.mcpProvider = a.provider
	if cfg.MCPIssuerURL != cfg.IssuerURL {
		a.mcpProvider, err = oidc.NewProvider(discoveryCtx, cfg.MCPIssuerURL)
		if err != nil {
			return nil, fmt.Errorf("discover MCP issuer: %w", err)
		}
	}
	a.verifier = a.provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	var endpoints struct {
		UserInfo string `json:"userinfo_endpoint"`
	}
	if err := a.provider.Claims(&endpoints); err != nil || endpoints.UserInfo == "" {
		return nil, fmt.Errorf("OIDC provider must advertise a userinfo endpoint")
	}
	a.userInfoURL = endpoints.UserInfo
	a.mcpVerifier = a.mcpProvider.Verifier(&oidc.Config{ClientID: cfg.PublicURL + "/mcp"})
	a.oauth = oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: a.provider.Endpoint(), RedirectURL: cfg.PublicURL + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email", oidc.ScopeOfflineAccess}}
	return a, nil
}

func (a *Auth) cookieName(kind string) string {
	if strings.HasPrefix(a.cfg.PublicURL, "https://") {
		return "__Host-beancount-" + kind
	}
	return "beancount-" + kind
}
func (a *Auth) cookie(w http.ResponseWriter, kind, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(kind), Value: value, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func key(value string) [32]byte { return sha256.Sum256([]byte(value)) }
func (a *Auth) pruneLocked() {
	now := time.Now()
	for k, s := range a.sessions {
		if (!s.renewable() && !now.Before(s.Expires)) || (s.renewable() && !now.Before(s.RetainUntil)) {
			delete(a.sessions, k)
		}
	}
	for id, expiry := range a.logoutSeen {
		if !now.Before(expiry) {
			delete(a.logoutSeen, id)
		}
	}
	for k, p := range a.pending {
		if !now.Before(p.Expires) {
			delete(a.pending, k)
		}
	}
}
func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	id := rand.Text()
	p := pending{State: rand.Text(), Nonce: rand.Text(), Verifier: oauth2.GenerateVerifier(), Expires: time.Now().Add(pendingTTL)}
	a.mu.Lock()
	a.pruneLocked()
	if len(a.pending) >= maxEntries {
		a.mu.Unlock()
		http.Error(w, "Too many login attempts", 429)
		return
	}
	if c, err := r.Cookie(a.cookieName("login")); err == nil {
		delete(a.pending, key(c.Value))
	}
	a.pending[key(id)] = p
	a.mu.Unlock()
	a.cookie(w, "login", id, int(pendingTTL.Seconds()))
	http.Redirect(w, r, a.oauth.AuthCodeURL(p.State, oidc.Nonce(p.Nonce), oauth2.S256ChallengeOption(p.Verifier)), http.StatusFound)
}
func (a *Auth) failed(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/?auth_error="+reason, http.StatusSeeOther)
}
func (a *Auth) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(a.cookieName("login"))
	if err != nil {
		http.Error(w, "Invalid login attempt", 400)
		return
	}
	a.mu.Lock()
	p, ok := a.pending[key(c.Value)]
	if ok && time.Now().Before(p.Expires) && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(p.State)) == 1 {
		delete(a.pending, key(c.Value))
	} else {
		a.mu.Unlock()
		http.Error(w, "Invalid or expired login attempt", 400)
		return
	}
	a.mu.Unlock()
	a.cookie(w, "login", "", -1)
	if r.URL.Query().Get("error") != "" {
		a.failed(w, r, "access_denied")
		return
	}
	if r.URL.Query().Get("code") == "" {
		a.failed(w, r, "login_failed")
		return
	}
	ctx := oidc.ClientContext(r.Context(), a.client)
	token, err := a.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(p.Verifier))
	if err != nil {
		a.failed(w, r, "login_failed")
		return
	}
	raw, _ := token.Extra("id_token").(string)
	id, err := a.verifier.Verify(ctx, raw)
	if err != nil || id.Subject == "" || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(p.Nonce)) != 1 {
		a.failed(w, r, "login_failed")
		return
	}
	if id.AccessTokenHash != "" && id.VerifyAccessToken(token.AccessToken) != nil {
		a.failed(w, r, "login_failed")
		return
	}
	var user User
	if id.Claims(&user) != nil {
		a.failed(w, r, "login_failed")
		return
	}
	expiry := time.Now().Add(a.cfg.SessionTTL)
	if id.Expiry.Before(expiry) {
		expiry = id.Expiry
	}
	if !token.Expiry.IsZero() && token.Expiry.Before(expiry) {
		expiry = token.Expiry
	}
	if !expiry.After(time.Now()) {
		a.failed(w, r, "login_failed")
		return
	}
	sid := rand.Text()
	a.mu.Lock()
	a.pruneLocked()
	if len(a.sessions) >= maxEntries {
		a.mu.Unlock()
		http.Error(w, "Session capacity reached", 503)
		return
	}
	if old, err := r.Cookie(a.cookieName("session")); err == nil {
		delete(a.sessions, key(old.Value))
	}
	var claims struct {
		SID string `json:"sid"`
	}
	_ = id.Claims(&claims)
	s := session{User: user, Expires: expiry, Token: token, SID: claims.SID, Nonce: p.Nonce, RefreshMu: &sync.Mutex{}, RetainUntil: time.Now().Add(sessionRetention), CheckAfter: time.Now().Add(time.Minute)}
	a.sessions[key(sid)] = s
	a.mu.Unlock()
	a.sessionCookie(w, sid, s)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != a.cfg.PublicURL {
		http.Error(w, "Invalid origin", 403)
		return
	}
	if c, err := r.Cookie(a.cookieName("session")); err == nil {
		a.mu.Lock()
		delete(a.sessions, key(c.Value))
		a.mu.Unlock()
	}
	a.cookie(w, "session", "", -1)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (a *Auth) metadata(w http.ResponseWriter) {
	jsonResponse(w, 200, map[string]any{"resource": a.cfg.PublicURL + "/mcp", "authorization_servers": []string{a.cfg.MCPIssuerURL}, "scopes_supported": []string{"openid", mcpScope, WriteScope}, "bearer_methods_supported": []string{"header"}})
}
func (a *Auth) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp", scope="openid %s"`, a.cfg.PublicURL, mcpScope))
	http.Error(w, "A valid MCP access token is required", http.StatusUnauthorized)
}
func (a *Auth) bearer(r *http.Request) bool {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 16384 {
		return false
	}
	ctx := oidc.ClientContext(r.Context(), a.client)
	id, err := a.mcpVerifier.Verify(ctx, parts[1])
	if err != nil || id.Subject == "" {
		return false
	}
	var claims struct {
		Scope     string `json:"scope"`
		NotBefore int64  `json:"nbf"`
	}
	if id.Claims(&claims) != nil || claims.NotBefore > time.Now().Unix() {
		return false
	}
	scopes := strings.Fields(claims.Scope)
	hasRead, hasOpenID := false, false
	for _, scope := range scopes {
		hasRead = hasRead || scope == mcpScope
		hasOpenID = hasOpenID || scope == "openid"
	}
	if !hasRead || !hasOpenID {
		return false
	}
	// UserInfo accepts access tokens only. This rejects ID/refresh tokens and
	// checks revocation at the issuer instead of trusting JWT expiry alone.
	info, err := a.mcpProvider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: parts[1], TokenType: "Bearer"}))
	if err != nil || info.Subject != id.Subject {
		return false
	}
	canWrite := false
	for _, scope := range scopes {
		canWrite = canWrite || scope == WriteScope
	}
	*r = *r.WithContext(WithAccess(r.Context(), Access{Authenticated: true, AuthEnabled: true, CanWrite: canWrite, Actor: "mcp:" + id.Subject, Expires: id.Expiry}))
	return true
}

func (a *Auth) Wrap(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(RPCPrefix, a.rpc(next))
	if !a.cfg.Disabled {
		mux.HandleFunc("GET /auth/login", a.login)
		mux.HandleFunc("GET /auth/callback", a.callback)
		mux.HandleFunc("POST /auth/logout", a.logout)
		mux.HandleFunc("POST /auth/backchannel-logout", a.backchannelLogout)
		mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) { a.metadata(w) })
		mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) { a.metadata(w) })
	}
	mux.HandleFunc("/auth/", http.NotFound)
	mux.HandleFunc("/.well-known/", http.NotFound)
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.Disabled && !a.bearer(r) {
			a.unauthorized(w)
			return
		}
		if a.cfg.Disabled {
			r = r.WithContext(WithAccess(r.Context(), a.browserAccess(r)))
		}
		access := FromContext(r.Context())
		if !access.Expires.IsZero() {
			ctx, cancel := context.WithDeadline(r.Context(), access.Expires)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
	mux.Handle("/", next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/auth/") || strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, RPCPrefix) || r.URL.Path == "/mcp" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !a.cfg.Disabled {
			origin, _ := url.Parse(a.cfg.PublicURL)
			// Never construct callbacks or metadata from forwarded headers / request Host.
			if r.URL.Path != "/healthz" && r.Host != origin.Host {
				http.Error(w, "Unrecognized host", 421)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
