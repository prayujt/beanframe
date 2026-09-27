package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func forceRenewal(f *fixture, c *http.Cookie) {
	f.a.mu.Lock()
	defer f.a.mu.Unlock()
	s := f.a.sessions[key(c.Value)]
	s.Expires = time.Now().Add(-time.Second)
	s.CheckAfter = time.Time{}
	f.a.sessions[key(c.Value)] = s
}

func TestSessionRefreshRotationAndConcurrency(t *testing.T) {
	f := setup(t)
	f.refresh = true
	f.active["new-access"] = "user-1"
	var calls atomic.Int32
	f.refreshHandler = func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		want := "refresh-1"
		if n > 1 {
			want = "refresh-2"
		}
		if r.Form.Get("refresh_token") != want {
			jsonResponse(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		claims := f.claims("browser")
		claims["name"] = "Updated name"
		jsonResponse(w, 200, map[string]any{"access_token": "new-access", "refresh_token": "refresh-2", "expires_in": 300, "token_type": "Bearer", "id_token": f.jwt(t, claims)})
	}
	c := f.login(t)
	if c.MaxAge <= 900 {
		t.Fatal("refreshable cookie still ends in 15 minutes")
	}
	forceRenewal(f, c)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil)
			if w.Code != 200 {
				t.Errorf("renewal: %d", w.Code)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent refresh count: %d", calls.Load())
	}
	w := f.request("POST", RPCPrefix+"GetSession", c, nil)
	if !strings.Contains(w.Body.String(), "Updated name") || strings.Contains(w.Body.String(), "refresh-2") {
		t.Fatal("claims not renewed or tokens exposed")
	}
	forceRenewal(f, c)
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 200 || calls.Load() != 2 {
		t.Fatal("rotated refresh token was not retained")
	}
}

func TestRefreshRejectsInvalidGrantAndChangedIdentity(t *testing.T) {
	for _, mode := range []string{"revoked", "subject", "audience", "nonce", "userinfo"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			f.refresh = true
			f.active["new-access"] = "user-1"
			f.refreshHandler = func(w http.ResponseWriter, r *http.Request) {
				if mode == "revoked" {
					jsonResponse(w, 400, map[string]string{"error": "invalid_grant"})
					return
				}
				claims := f.claims("browser")
				switch mode {
				case "subject":
					claims["sub"] = "other"
				case "audience":
					claims["aud"] = "other"
				case "nonce":
					claims["nonce"] = "other"
				}
				access := "new-access"
				if mode == "userinfo" {
					access = "revoked"
				}
				jsonResponse(w, 200, map[string]any{"access_token": access, "refresh_token": "refresh-2", "expires_in": 300, "token_type": "Bearer", "id_token": f.jwt(t, claims)})
			}
			c := f.login(t)
			forceRenewal(f, c)
			if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 401 {
				t.Fatal("invalid refresh allowed access")
			}
			f.a.mu.Lock()
			_, exists := f.a.sessions[key(c.Value)]
			f.a.mu.Unlock()
			if exists {
				t.Fatal("invalid session retained")
			}
		})
	}
}

func TestProviderRevocationBeforeTokenExpiry(t *testing.T) {
	f := setup(t)
	f.refresh = true
	c := f.login(t)
	f.a.mu.Lock()
	s := f.a.sessions[key(c.Value)]
	s.CheckAfter = time.Time{}
	f.a.sessions[key(c.Value)] = s
	f.a.mu.Unlock()
	// The access JWT still has time left, but the issuer rejects UserInfo.
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 401 {
		t.Fatal("provider revocation ignored")
	}
}

func TestRefreshOutageDoesNotExtendExpiredAccess(t *testing.T) {
	f := setup(t)
	f.refresh = true
	f.refreshHandler = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }
	c := f.login(t)
	f.a.mu.Lock()
	s := f.a.sessions[key(c.Value)]
	s.Expires = time.Now().Add(30 * time.Second)
	s.CheckAfter = time.Time{}
	f.a.sessions[key(c.Value)] = s
	f.a.mu.Unlock()
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 200 {
		t.Fatal("temporary outage ended a valid session")
	}
	forceRenewal(f, c)
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 401 {
		t.Fatal("outage extended expired access")
	}
}

func (f *fixture) sendLogout(token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "https://books.example/auth/backchannel-logout", strings.NewReader(url.Values{"logout_token": {token}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.a.Wrap(http.NotFoundHandler()).ServeHTTP(w, r)
	return w
}

func TestBackchannelLogoutValidationAndScope(t *testing.T) {
	f := setup(t)
	c := f.login(t)
	valid := func() map[string]any {
		return map[string]any{"iss": f.issuer.URL, "aud": "browser", "iat": time.Now().Unix(), "jti": "logout-1", "sid": "provider-session", "events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}}}
	}
	for _, mode := range []string{"audience", "issuer", "nonce", "event", "old", "future", "no-session", "no-jti"} {
		claims := valid()
		switch mode {
		case "audience":
			claims["aud"] = "other"
		case "issuer":
			claims["iss"] = "https://attacker.example"
		case "nonce":
			claims["nonce"] = ""
		case "event":
			delete(claims, "events")
		case "old":
			claims["iat"] = time.Now().Add(-10 * time.Minute).Unix()
		case "future":
			claims["iat"] = time.Now().Add(10 * time.Minute).Unix()
		case "no-session":
			delete(claims, "sid")
		case "no-jti":
			delete(claims, "jti")
		}
		if w := f.sendLogout(f.jwt(t, claims)); w.Code != 400 {
			t.Errorf("accepted %s logout", mode)
		}
	}
	claims := valid()
	claims["sid"] = "unrelated-session"
	claims["jti"] = "other"
	if w := f.sendLogout(f.jwt(t, claims)); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 200 {
		t.Fatal("unrelated provider session logged out")
	}
	token := f.jwt(t, valid())
	if w := f.sendLogout(token); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := f.request("POST", RPCPrefix+"GetSnapshot", c, nil); w.Code != 401 {
		t.Fatal("backchannel logout did not invalidate session")
	}
	if w := f.sendLogout(token); w.Code != 200 {
		t.Fatal("retry must be idempotent")
	}
}

func TestLogoutDuringRefreshCannotResurrectSession(t *testing.T) {
	f := setup(t)
	f.refresh = true
	f.active["new-access"] = "user-1"
	started, resume := make(chan struct{}), make(chan struct{})
	f.refreshHandler = func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-resume
		jsonResponse(w, 200, map[string]any{"access_token": "new-access", "refresh_token": "refresh-2", "expires_in": 300, "token_type": "Bearer"})
	}
	c := f.login(t)
	forceRenewal(f, c)
	done := make(chan int, 1)
	go func() { done <- f.request("POST", RPCPrefix+"GetSnapshot", c, nil).Code }()
	<-started
	f.request("POST", "/auth/logout", c, map[string]string{"Origin": "https://books.example"})
	close(resume)
	if code := <-done; code != 401 {
		t.Fatal("in-flight refresh resurrected logout")
	}
}
