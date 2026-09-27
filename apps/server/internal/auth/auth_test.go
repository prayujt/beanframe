package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type fixture struct {
	a                *Auth
	issuer           *httptest.Server
	signer           jose.Signer
	nonce, challenge string
	active           map[string]string
	refresh          bool
	refreshHandler   http.HandlerFunc
}

func setup(t *testing.T) *fixture {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: private}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{signer: signer, active: map[string]string{}}
	mux := http.NewServeMux()
	f.issuer = httptest.NewServer(mux)
	t.Cleanup(f.issuer.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, map[string]any{"issuer": f.issuer.URL, "authorization_endpoint": f.issuer.URL + "/authorize", "token_endpoint": f.issuer.URL + "/token", "userinfo_endpoint": f.issuer.URL + "/userinfo", "jwks_uri": f.issuer.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &private.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "refresh_token" {
			if f.refreshHandler != nil {
				f.refreshHandler(w, r)
			} else {
				jsonResponse(w, 400, map[string]string{"error": "invalid_grant"})
			}
			return
		}
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("code") != "valid" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("redirect_uri") != "https://books.example/auth/callback" {
			http.Error(w, "bad exchange", 400)
			return
		}
		claims := f.claims("browser")
		claims["nonce"] = f.nonce
		claims["name"] = "Test User"
		claims["preferred_username"] = "reader"
		claims["email"] = "reader@example.com"
		claims["groups"] = []string{"readers"}
		claims["sid"] = "provider-session"
		response := map[string]any{"access_token": "access", "token_type": "Bearer", "expires_in": 300, "id_token": f.jwt(t, claims)}
		if f.refresh {
			response["refresh_token"] = "refresh-1"
		}
		jsonResponse(w, 200, response)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		sub, ok := f.active[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		if !ok {
			http.Error(w, "invalid", 401)
			return
		}
		jsonResponse(w, 200, map[string]string{"sub": sub})
	})
	f.a, err = New(context.Background(), Config{PublicURL: "https://books.example", IssuerURL: f.issuer.URL, ClientID: "browser", ProviderName: "Test OIDC", SessionTTL: 15 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) claims(audience string) map[string]any {
	return map[string]any{"iss": f.issuer.URL, "sub": "user-1", "aud": audience, "iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix()}
}
func (f *fixture) jwt(t *testing.T, claims map[string]any) string {
	t.Helper()
	body, _ := json.Marshal(claims)
	signed, err := f.signer.Sign(body)
	if err != nil {
		t.Fatal(err)
	}
	token, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func (f *fixture) request(method, path string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://books.example"+path, nil)
	if strings.HasPrefix(path, RPCPrefix) {
		r.Header.Set("Origin", "https://books.example")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, RPCPrefix) {
			a := FromContext(r.Context())
			jsonResponse(w, 200, map[string]any{"authenticated": a.Authenticated, "user": a.User})
			return
		}
		w.WriteHeader(204)
	})).ServeHTTP(w, r)
	return w
}
func (f *fixture) start(t *testing.T) (*http.Cookie, string) {
	t.Helper()
	w := f.request("GET", "/auth/login", nil, nil)
	if w.Code != 302 {
		t.Fatal(w.Code)
	}
	u, _ := url.Parse(w.Header().Get("Location"))
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatal("missing login protections")
	}
	f.nonce = q.Get("nonce")
	f.challenge = q.Get("code_challenge")
	c := w.Result().Cookies()[0]
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Domain != "" || !strings.HasPrefix(c.Name, "__Host-") {
		t.Fatalf("unsafe cookie: %+v", c)
	}
	return c, q.Get("state")
}
func (f *fixture) login(t *testing.T) *http.Cookie {
	t.Helper()
	c, state := f.start(t)
	w := f.request("GET", "/auth/callback?code=valid&state="+state, c, nil)
	if w.Code != 303 || w.Header().Get("Location") != "/" {
		t.Fatalf("login: %d %s", w.Code, w.Header().Get("Location"))
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == f.a.cookieName("session") {
			return cookie
		}
	}
	t.Fatal("no session cookie")
	return nil
}
func TestLoginSessionLogout(t *testing.T) {
	f := setup(t)
	if w := f.request("POST", RPCPrefix+"GetSnapshot", nil, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	c := f.login(t)
	w := f.request("POST", RPCPrefix+"GetSession", c, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"authenticated":true`) || !strings.Contains(w.Body.String(), "reader@example.com") || strings.Contains(w.Body.String(), "access_token") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Body.String())
	}
	if w = f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = f.request("POST", "/mcp", c, nil); w.Code != 401 {
		t.Fatal("browser cookie authorized MCP")
	}
	if w = f.request("GET", "/auth/logout", c, nil); w.Code == 303 {
		t.Fatal("GET logged out")
	}
	if w = f.request("POST", "/auth/logout", c, map[string]string{"Origin": "https://attacker.example"}); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = f.request("POST", "/auth/logout", c, map[string]string{"Origin": "https://books.example"}); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if w = f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 401 {
		t.Fatal("logout did not invalidate session")
	}
}
func TestCallbackRejection(t *testing.T) {
	f := setup(t)
	t.Run("state and replay", func(t *testing.T) {
		c, state := f.start(t)
		if w := f.request("GET", "/auth/callback?code=valid&state=wrong", c, nil); w.Code != 400 {
			t.Fatal(w.Code)
		}
		path := "/auth/callback?error=access_denied&state=" + state
		w := f.request("GET", path, c, nil)
		if w.Header().Get("Location") != "/?auth_error=access_denied" {
			t.Fatal(w.Header())
		}
		if w = f.request("GET", path, c, nil); w.Code != 400 {
			t.Fatal("callback replay accepted")
		}
	})
	t.Run("nonce", func(t *testing.T) {
		c, state := f.start(t)
		f.nonce = "wrong"
		if w := f.request("GET", "/auth/callback?code=valid&state="+state, c, nil); w.Header().Get("Location") != "/?auth_error=login_failed" {
			t.Fatal(w.Header())
		}
	})
	t.Run("expired attempt", func(t *testing.T) {
		c, state := f.start(t)
		p := f.a.pending[key(c.Value)]
		p.Expires = time.Now().Add(-time.Second)
		f.a.pending[key(c.Value)] = p
		if w := f.request("GET", "/auth/callback?code=valid&state="+state, c, nil); w.Code != 400 {
			t.Fatal(w.Code)
		}
	})
}
func TestExpiredAndForgedSession(t *testing.T) {
	f := setup(t)
	c := f.login(t)
	f.a.mu.Lock()
	s := f.a.sessions[key(c.Value)]
	s.Expires = time.Now().Add(-time.Second)
	f.a.sessions[key(c.Value)] = s
	f.a.mu.Unlock()
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	c.Value = "forged"
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestMCPBearerValidation(t *testing.T) {
	f := setup(t)
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		active bool
		sub    string
		want   int
	}{
		{"valid", func(c map[string]any) {}, true, "user-1", 204},
		{"wrong audience", func(c map[string]any) { c["aud"] = "another-app" }, true, "user-1", 401},
		{"wrong issuer", func(c map[string]any) { c["iss"] = "https://attacker.example" }, true, "user-1", 401},
		{"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, true, "user-1", 401},
		{"future nbf", func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() }, true, "user-1", 401},
		{"missing scope", func(c map[string]any) { delete(c, "scope") }, true, "user-1", 401},
		{"wrong scope", func(c map[string]any) { c["scope"] = "openid unrelated:read" }, true, "user-1", 401},
		{"revoked or ID token", func(c map[string]any) {}, false, "user-1", 401},
		{"userinfo mismatch", func(c map[string]any) {}, true, "someone-else", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := f.claims("https://books.example/mcp")
			claims["scope"] = "openid " + mcpScope
			tc.change(claims)
			token := f.jwt(t, claims)
			delete(f.active, token)
			if tc.active {
				f.active[token] = tc.sub
			}
			w := f.request("POST", "/mcp", nil, map[string]string{"Authorization": "Bearer " + token})
			if w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
	if w := f.request("POST", "/mcp", nil, map[string]string{"Authorization": "Bearer not-a-token"}); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := f.request("POST", "/mcp", nil, nil)
	if !strings.Contains(w.Header().Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatal(w.Header())
	}
	w = f.request("GET", "/.well-known/oauth-protected-resource/mcp", nil, nil)
	if !strings.Contains(w.Body.String(), f.issuer.URL) || !strings.Contains(w.Body.String(), "https://books.example/mcp") {
		t.Fatal(w.Body.String())
	}
	// Host and forwarding headers must not select the callback or token audience.
	r := httptest.NewRequest("GET", "https://attacker.example/auth/login", nil)
	w = httptest.NewRecorder()
	f.a.Wrap(http.NotFoundHandler()).ServeHTTP(w, r)
	if w.Code != 421 {
		t.Fatal(w.Code)
	}
}
func TestConfiguration(t *testing.T) {
	valid := Config{PublicURL: "https://books.example", IssuerURL: "https://id.example/application/", ClientID: "client", SessionTTL: 15 * time.Minute}
	for i, change := range []func(*Config){func(c *Config) { c.PublicURL = "" }, func(c *Config) { c.IssuerURL = "http://id.example" }, func(c *Config) { c.PublicURL = "https://user:pass@books.example" }, func(c *Config) { c.PublicURL = "https://books.example/path" }, func(c *Config) { c.ClientID = "" }, func(c *Config) { c.SessionTTL = 2 * time.Hour }, func(c *Config) { c.Disabled = true }} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			c := valid
			change(&c)
			if c.Validate() == nil {
				t.Fatal("accepted unsafe config")
			}
		})
	}
	if valid.Validate() != nil {
		t.Fatal("rejected valid config")
	}
	if _, err := New(context.Background(), Config{}); err == nil {
		t.Fatal("missing credentials did not fail closed")
	}
	if _, err := New(context.Background(), Config{Disabled: true}); err != nil {
		t.Fatal(err)
	}
}

func TestRPCSessionOriginAndScopeBoundaries(t *testing.T) {
	f := setup(t)
	c := f.login(t)
	for _, origin := range []string{"", "https://foreign.example"} {
		w := f.request("POST", RPCPrefix+"SaveTransaction", c, map[string]string{"Origin": origin})
		if w.Code != 403 {
			t.Fatalf("accepted origin %q: %d", origin, w.Code)
		}
	}
	if w := f.request("POST", RPCPrefix+"SaveTransaction", nil, nil); w.Code != 401 {
		t.Fatal("anonymous RPC write accepted")
	}
	if w := f.request("POST", RPCPrefix+"GetSession", nil, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"authenticated":false`) {
		t.Fatal("public session unavailable")
	}
	if w := f.request("GET", "/auth/session", c, nil); w.Code != 404 {
		t.Fatal("legacy session route remains")
	}
	if w := f.request("GET", "/api/me", c, nil); w.Code != 404 {
		t.Fatal("legacy identity route remains")
	}
	r := httptest.NewRequest("POST", "https://books.example"+RPCPrefix+"WatchLedger", nil)
	r.AddCookie(c)
	access := f.a.browserAccess(r)
	if !access.Valid() || !access.CanWrite {
		t.Fatal("valid browser access missing")
	}
	f.request("POST", "/auth/logout", c, map[string]string{"Origin": "https://books.example"})
	if access.Valid() {
		t.Fatal("stream access survived logout")
	}
	for _, write := range []bool{false, true} {
		claims := f.claims("https://books.example/mcp")
		claims["scope"] = "openid " + mcpScope
		if write {
			claims["scope"] = claims["scope"].(string) + " " + WriteScope
		}
		token := f.jwt(t, claims)
		f.active[token] = "user-1"
		r := httptest.NewRequest("POST", "https://books.example/mcp", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if !f.a.bearer(r) {
			t.Fatal("valid MCP token rejected")
		}
		if FromContext(r.Context()).CanWrite != write {
			t.Fatal("write scope was not enforced")
		}
	}
}
