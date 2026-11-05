package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig describes the identity provider (Authentik) sign-in.
type OIDCConfig struct {
	Issuer         string
	ClientID       string
	ClientSecret   string
	RedirectURL    string
	Group          string
	MaxPeers       int
	PeerTTL        time.Duration
	ReconcileEvery time.Duration
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// secureURL accepts https URLs, and http only for loopback development.
func secureURL(name, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%s must be an absolute URL, got %q", name, raw)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return nil, fmt.Errorf("%s must use https (http is only allowed for localhost)", name)
	}
	return u, nil
}

func loadOIDC(lookup func(string) string, cfg *Config) error {
	issuer := getenv(lookup, "VPN_OIDC_ISSUER", "")
	if issuer == "" {
		return nil
	}
	o := &OIDCConfig{
		Issuer:       issuer,
		ClientID:     getenv(lookup, "VPN_OIDC_CLIENT_ID", ""),
		ClientSecret: getenv(lookup, "VPN_OIDC_CLIENT_SECRET", ""),
		RedirectURL:  getenv(lookup, "VPN_OIDC_REDIRECT_URL", ""),
		Group:        getenv(lookup, "VPN_OIDC_GROUP", "vpn-users"),
	}
	if _, err := secureURL("VPN_OIDC_ISSUER", o.Issuer); err != nil {
		return err
	}
	if o.ClientID == "" {
		return errors.New("VPN_OIDC_CLIENT_ID is required with VPN_OIDC_ISSUER")
	}
	ru, err := secureURL("VPN_OIDC_REDIRECT_URL", o.RedirectURL)
	if err != nil {
		return err
	}
	if ru.Path != "/auth/callback" {
		return errors.New("VPN_OIDC_REDIRECT_URL must end in /auth/callback")
	}

	maxPeers := getenv(lookup, "VPN_MAX_PEERS_PER_USER", "5")
	if o.MaxPeers, err = strconv.Atoi(maxPeers); err != nil || o.MaxPeers < 1 {
		return fmt.Errorf("VPN_MAX_PEERS_PER_USER must be a positive number, got %q", maxPeers)
	}
	if o.PeerTTL, err = time.ParseDuration(getenv(lookup, "VPN_PEER_TTL", "0")); err != nil || o.PeerTTL < 0 {
		return errors.New("VPN_PEER_TTL must be a non-negative duration such as 720h")
	}
	if o.ReconcileEvery, err = time.ParseDuration(getenv(lookup, "VPN_RECONCILE_INTERVAL", "5m")); err != nil || o.ReconcileEvery < 30*time.Second {
		return errors.New("VPN_RECONCILE_INTERVAL must be a duration of at least 30s")
	}
	cfg.OIDC = o
	return nil
}

const (
	sessionCookie = "vpn_session"
	stateCookie   = "vpn_login"
	sessionTTL    = 8 * time.Hour
	loginTTL      = 10 * time.Minute
	maxPending    = 1000
)

type user struct{ Sub, Name string }

type session struct {
	user    user
	expires time.Time
}

type pendingLogin struct {
	verifier, nonce string
	expires         time.Time
}

type idClaims struct {
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Groups            []string `json:"groups"`
}

// Authenticator signs users in with OIDC (authorization code + PKCE) and
// keeps their peers tied to their account in the identity provider.
type Authenticator struct {
	cfg      OIDCConfig
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	secure   bool
	origin   string
	api      *api
	owners   *OwnerStore
	now      func() time.Time

	mu       sync.Mutex
	pending  map[string]pendingLogin
	sessions map[string]session
}

// NewAuthenticator discovers the provider. ctx must outlive the process
// lifetime of the verifier: it is reused to fetch signing keys.
func NewAuthenticator(ctx context.Context, cfg OIDCConfig, a *api, owners *OwnerStore) (*Authenticator, error) {
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	ru, err := url.Parse(cfg.RedirectURL)
	if err != nil {
		return nil, err
	}
	return &Authenticator{
		cfg: cfg,
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email", oidc.ScopeOfflineAccess},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		secure:   ru.Scheme == "https",
		origin:   ru.Scheme + "://" + ru.Host,
		api:      a,
		owners:   owners,
		now:      time.Now,
		pending:  map[string]pendingLogin{},
		sessions: map[string]session{},
	}, nil
}

func (a *Authenticator) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", a.login)
	mux.HandleFunc("GET /auth/callback", a.callback)
	mux.Handle("POST /auth/logout", a.require(a.logout))
	mux.Handle("GET /api/v1/me", a.require(a.me))
	mux.Handle("GET /api/v1/me/peers", a.require(a.listMine))
	mux.Handle("POST /api/v1/me/peers", a.require(a.createMine))
	mux.Handle("DELETE /api/v1/me/peers/{id}", a.require(a.deleteMine))
}

func randToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system CSPRNG failing is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *Authenticator) cookie(name, value, path string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value, Path: path, MaxAge: maxAge,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	}
}

func (a *Authenticator) login(w http.ResponseWriter, r *http.Request) {
	state, nonce, verifier := randToken(), randToken(), oauth2.GenerateVerifier()

	a.mu.Lock()
	now := a.now()
	for k, p := range a.pending {
		if now.After(p.expires) {
			delete(a.pending, k)
		}
	}
	if len(a.pending) >= maxPending {
		a.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "too many pending sign-ins")
		return
	}
	a.pending[state] = pendingLogin{verifier: verifier, nonce: nonce, expires: now.Add(loginTTL)}
	a.mu.Unlock()

	// The cookie ties the callback to this browser, defeating login CSRF.
	http.SetCookie(w, a.cookie(stateCookie, state, "/auth", int(loginTTL.Seconds())))
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (a *Authenticator) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	c, err := r.Cookie(stateCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		writeError(w, http.StatusBadRequest, "sign-in state mismatch; start again at /auth/login")
		return
	}
	http.SetCookie(w, a.cookie(stateCookie, "", "/auth", -1))

	a.mu.Lock()
	pl, ok := a.pending[state]
	delete(a.pending, state)
	a.mu.Unlock()
	if !ok || a.now().After(pl.expires) {
		writeError(w, http.StatusBadRequest, "sign-in expired; start again at /auth/login")
		return
	}
	if q.Get("error") != "" || q.Get("code") == "" {
		writeError(w, http.StatusUnauthorized, "sign-in was not completed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tok, err := a.oauth.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(pl.verifier))
	if err != nil {
		log.Printf("oidc: code exchange failed: %v", err)
		writeError(w, http.StatusUnauthorized, "sign-in failed")
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := a.verifier.Verify(ctx, raw)
	if err != nil || idt.Subject == "" || idt.Nonce != pl.nonce {
		log.Printf("oidc: id token rejected: %v", err)
		writeError(w, http.StatusUnauthorized, "sign-in failed")
		return
	}
	var claims idClaims
	if err := idt.Claims(&claims); err != nil {
		writeError(w, http.StatusUnauthorized, "sign-in failed")
		return
	}
	if !slices.Contains(claims.Groups, a.cfg.Group) {
		writeError(w, http.StatusForbidden, "your account is not a member of the required group")
		return
	}

	name := claims.PreferredUsername
	if name == "" {
		name = claims.Email
	}
	if name == "" {
		name = idt.Subject
	}
	if err := a.owners.Set(idt.Subject, name, tok.RefreshToken); err != nil {
		log.Printf("owners: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tok.RefreshToken == "" {
		log.Printf("oidc: no refresh token for %s; revocation relies on VPN_PEER_TTL (enable offline_access)", name)
	}

	sid := randToken()
	a.mu.Lock()
	a.sessions[sid] = session{user: user{Sub: idt.Subject, Name: name}, expires: a.now().Add(sessionTTL)}
	a.mu.Unlock()
	http.SetCookie(w, a.cookie(sessionCookie, sid, "/", int(sessionTTL.Seconds())))
	http.Redirect(w, r, "/api/v1/me", http.StatusSeeOther)
}

func (a *Authenticator) current(r *http.Request) (user, string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return user{}, "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[c.Value]
	if !ok || a.now().After(s.expires) {
		delete(a.sessions, c.Value)
		return user{}, "", false
	}
	return s.user, c.Value, true
}

// sameOrigin blocks cross-site state changes made with the session cookie.
func (a *Authenticator) sameOrigin(r *http.Request) bool {
	if o := r.Header.Get("Origin"); o != "" {
		return o == a.origin
	}
	return r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

func (a *Authenticator) require(next func(http.ResponseWriter, *http.Request, user, string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, sid, ok := a.current(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "sign in at /auth/login")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !a.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "cross-origin request refused")
			return
		}
		next(w, r, u, sid)
	})
}

func (a *Authenticator) logout(w http.ResponseWriter, _ *http.Request, _ user, sid string) {
	a.mu.Lock()
	delete(a.sessions, sid)
	a.mu.Unlock()
	http.SetCookie(w, a.cookie(sessionCookie, "", "/", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (a *Authenticator) me(w http.ResponseWriter, _ *http.Request, u user, _ string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"subject":  u.Sub,
		"name":     u.Name,
		"maxPeers": a.cfg.MaxPeers,
		"peers":    a.api.svc.ListOwned(u.Sub),
	})
}

func (a *Authenticator) listMine(w http.ResponseWriter, _ *http.Request, u user, _ string) {
	writeJSON(w, http.StatusOK, a.api.svc.ListOwned(u.Sub))
}

func (a *Authenticator) createMine(w http.ResponseWriter, r *http.Request, u user, _ string) {
	a.api.create(w, r, CreateOptions{Owner: u.Sub, OwnerName: u.Name, TTL: a.cfg.PeerTTL, MaxPerOwner: a.cfg.MaxPeers})
}

func (a *Authenticator) deleteMine(w http.ResponseWriter, r *http.Request, u user, _ string) {
	if err := a.api.svc.DeleteOwned(r.Context(), r.PathValue("id"), u.Sub); err != nil {
		a.api.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Reconcile removes expired peers and revokes the peers of users who were
// deactivated or left the required group. Transient provider errors keep
// peers in place so an outage cannot lock everyone out.
func (a *Authenticator) Reconcile(ctx context.Context) {
	if n, err := a.api.svc.RemoveExpired(ctx, a.now()); err != nil {
		log.Printf("reconcile: expiry: %v", err)
	} else if n > 0 {
		log.Printf("reconcile: removed %d expired peer(s)", n)
	}

	for sub, rec := range a.owners.Snapshot() {
		if rec.RefreshToken == "" || len(a.api.svc.ListOwned(sub)) == 0 {
			continue
		}
		if !a.stillAllowed(ctx, sub, rec) {
			a.revoke(ctx, sub, rec.Name)
		}
	}
}

func (a *Authenticator) stillAllowed(ctx context.Context, sub string, rec ownerRecord) bool {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	tok, err := a.oauth.TokenSource(ctx, &oauth2.Token{RefreshToken: rec.RefreshToken}).Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && (re.ErrorCode == "invalid_grant" ||
			(re.Response != nil && (re.Response.StatusCode == http.StatusBadRequest || re.Response.StatusCode == http.StatusUnauthorized))) {
			return false
		}
		log.Printf("reconcile: provider unreachable for %s, keeping peers: %v", rec.Name, err)
		return true
	}
	if tok.RefreshToken != "" && tok.RefreshToken != rec.RefreshToken {
		if err := a.owners.Set(sub, rec.Name, tok.RefreshToken); err != nil {
			log.Printf("owners: %v", err)
		}
	}

	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		log.Printf("reconcile: no id_token on refresh for %s; group not re-checked", rec.Name)
		return true
	}
	idt, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		log.Printf("reconcile: id token for %s did not verify, keeping peers: %v", rec.Name, err)
		return true
	}
	var claims idClaims
	if err := idt.Claims(&claims); err != nil {
		return true
	}
	return slices.Contains(claims.Groups, a.cfg.Group)
}

func (a *Authenticator) revoke(ctx context.Context, sub, name string) {
	n, err := a.api.svc.RemoveOwner(ctx, sub)
	if err != nil {
		log.Printf("reconcile: revoking %s: %v", name, err)
		return
	}
	if err := a.owners.Delete(sub); err != nil {
		log.Printf("owners: %v", err)
	}
	a.mu.Lock()
	for id, s := range a.sessions {
		if s.user.Sub == sub {
			delete(a.sessions, id)
		}
	}
	a.mu.Unlock()
	log.Printf("reconcile: %s is no longer allowed; removed %d peer(s)", name, n)
}

// RunReconciler checks on a fixed interval until ctx ends.
func (a *Authenticator) RunReconciler(ctx context.Context) {
	t := time.NewTicker(a.cfg.ReconcileEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Reconcile(ctx)
		}
	}
}
