package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Browser cookies have a rolling lifetime; the provider still controls whether
// tokens may renew. This is also the retention bound for abandoned sessions.
const sessionRetention = 400 * 24 * time.Hour

func (s session) renewable() bool { return s.Token != nil && s.Token.RefreshToken != "" }

func (a *Auth) sessionCookie(w http.ResponseWriter, id string, s session) {
	age := int(time.Until(s.Expires).Seconds())
	if s.renewable() {
		age = int(sessionRetention.Seconds())
	}
	if age < 1 {
		age = 1
	}
	a.cookie(w, "session", id, age)
}

func (a *Auth) current(r *http.Request) (session, bool) {
	c, err := r.Cookie(a.cookieName("session"))
	if err != nil {
		return session{}, false
	}
	k := key(c.Value)
	a.mu.Lock()
	s, ok := a.sessions[k]
	a.mu.Unlock()
	if !ok {
		return session{}, false
	}
	// Each browser session serializes refreshes so concurrent RPCs/tabs cannot
	// reuse a rotated refresh token. Other users and logout are never blocked.
	s.RefreshMu.Lock()
	defer s.RefreshMu.Unlock()
	a.mu.Lock()
	s, ok = a.sessions[k]
	a.mu.Unlock()
	if !ok {
		return session{}, false
	}
	now := time.Now()
	if !s.renewable() {
		if now.Before(s.Expires) {
			return s, true
		}
		a.mu.Lock()
		delete(a.sessions, k)
		a.mu.Unlock()
		return session{}, false
	}
	if !now.Before(s.RetainUntil) {
		a.mu.Lock()
		delete(a.sessions, k)
		a.mu.Unlock()
		return session{}, false
	}
	if now.Before(s.CheckAfter) {
		return s, now.Before(s.Expires)
	}
	ctx := oidc.ClientContext(r.Context(), a.client)
	next := s
	terminal := false
	if !now.Before(s.Expires.Add(-time.Minute)) {
		old := *s.Token
		old.Expiry = now.Add(-time.Second)
		next.Token, err = a.oauth.TokenSource(ctx, &old).Token()
		if err != nil {
			var failure *oauth2.RetrieveError
			terminal = errors.As(err, &failure) && failure.Response != nil && failure.Response.StatusCode < 500 && failure.Response.StatusCode != 429
		} else {
			next.Expires = now.Add(a.cfg.SessionTTL)
			if next.Token.Expiry.IsZero() || next.Token.AccessToken == "" {
				terminal = true
			}
			if next.Token.Expiry.Before(next.Expires) {
				next.Expires = next.Token.Expiry
			}
			if raw, _ := next.Token.Extra("id_token").(string); raw != "" {
				id, verifyErr := a.verifier.Verify(ctx, raw)
				if verifyErr != nil || id.Subject != s.User.Subject || (id.Nonce != "" && id.Nonce != s.Nonce) || (id.AccessTokenHash != "" && id.VerifyAccessToken(next.Token.AccessToken) != nil) {
					terminal = true
				} else {
					if id.Claims(&next.User) != nil {
						terminal = true
					}
					if id.Expiry.Before(next.Expires) {
						next.Expires = id.Expiry
					}
				}
			}
			if !next.Expires.After(now) {
				terminal = true
			}
		}
	}
	if err == nil && !terminal {
		// Detect provider revocation even when the access token has not expired,
		// including on preview instances sharing one OIDC client.
		terminal, err = a.checkUserInfo(ctx, next.Token, s.User.Subject)
	}
	if err != nil {
		// Preserve a successfully rotated token even if UserInfo is temporarily
		// unavailable; its predecessor may already have been revoked.
		if next.Token == nil {
			next.Token = s.Token
		}
		next.Expires = s.Expires
		next.CheckAfter = now.Add(10 * time.Second)
	} else {
		next.CheckAfter = now.Add(time.Minute)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if current, exists := a.sessions[k]; !exists || current.RefreshMu != s.RefreshMu {
		return session{}, false
	}
	if terminal {
		delete(a.sessions, k)
		return session{}, false
	}
	next.RetainUntil = now.Add(sessionRetention)
	a.sessions[k] = next
	return next, now.Before(next.Expires)
}

func (a *Auth) checkUserInfo(ctx context.Context, token *oauth2.Token, subject string) (bool, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, a.userInfoURL, nil)
	if err != nil {
		return false, err
	}
	token.SetAuthHeader(r)
	response, err := a.client.Do(r)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return true, nil
	}
	if response.StatusCode != 200 {
		return false, fmt.Errorf("provider userinfo unavailable")
	}
	var user User
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&user); err != nil {
		return false, err
	}
	return user.Subject != subject, nil
}

func (a *Auth) backchannelLogout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if r.ParseForm() != nil {
		http.Error(w, "Invalid logout", 400)
		return
	}
	// Logout JWTs need not contain exp. Verify signature/issuer/audience with
	// discovery keys, then enforce freshness and the logout-specific claims.
	verifier := a.provider.Verifier(&oidc.Config{ClientID: a.cfg.ClientID, SkipExpiryCheck: true})
	id, err := verifier.Verify(oidc.ClientContext(r.Context(), a.client), r.PostForm.Get("logout_token"))
	if err != nil {
		http.Error(w, "Invalid logout", 400)
		return
	}
	var claims struct {
		SID    string                     `json:"sid"`
		JTI    string                     `json:"jti"`
		IAT    int64                      `json:"iat"`
		EXP    int64                      `json:"exp"`
		Nonce  json.RawMessage            `json:"nonce"`
		Events map[string]json.RawMessage `json:"events"`
	}
	if id.Claims(&claims) != nil {
		http.Error(w, "Invalid logout", 400)
		return
	}
	now := time.Now()
	var event map[string]any
	value := claims.Events["http://schemas.openid.net/event/backchannel-logout"]
	if claims.JTI == "" || (id.Subject == "" && claims.SID == "") || claims.Nonce != nil || claims.IAT < now.Add(-5*time.Minute).Unix() || claims.IAT > now.Add(time.Minute).Unix() || (claims.EXP != 0 && claims.EXP <= now.Unix()) || json.Unmarshal(value, &event) != nil || event == nil {
		http.Error(w, "Invalid logout", 400)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked()
	if _, seen := a.logoutSeen[claims.JTI]; seen {
		w.WriteHeader(200)
		return
	}
	if len(a.logoutSeen) >= maxEntries {
		http.Error(w, "Try again", 503)
		return
	}
	a.logoutSeen[claims.JTI] = now.Add(6 * time.Minute)
	for key, s := range a.sessions {
		if (claims.SID == "" || s.SID == claims.SID) && (id.Subject == "" || s.User.Subject == id.Subject) {
			delete(a.sessions, key)
		}
	}
	w.WriteHeader(200)
}
