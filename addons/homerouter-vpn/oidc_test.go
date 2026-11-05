package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	testClientID = "vpn-client"
	testOrigin   = "http://127.0.0.1:8081"
)

type codeInfo struct{ nonce, challenge, redirect string }

// mockIDP is a minimal OIDC provider that can misbehave on demand.
type mockIDP struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	signer jose.Signer

	mu       sync.Mutex
	groups   []string
	disabled bool // refresh fails with invalid_grant
	down     bool // token endpoint answers 500
	badNonce bool
	codes    map[string]codeInfo
}

func newMockIDP(t *testing.T) *mockIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	m := &mockIDP{t: t, key: key, signer: signer, groups: []string{"vpn-users"}, codes: map[string]codeInfo{}}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"issuer":                                m.srv.URL,
			"authorization_endpoint":                m.srv.URL + "/authorize",
			"token_endpoint":                        m.srv.URL + "/token",
			"jwks_uri":                              m.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/authorize", m.authorize)
	mux.HandleFunc("/token", m.token)
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockIDP) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("response_type") != "code" || q.Get("client_id") != testClientID ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" ||
		q.Get("state") == "" || q.Get("nonce") == "" {
		http.Error(w, "bad authorize request", 400)
		return
	}
	code := randToken()
	m.mu.Lock()
	m.codes[code] = codeInfo{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri")}
	m.mu.Unlock()
	http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
}

func (m *mockIDP) mint(nonce string) string {
	type claims struct {
		jwt.Claims
		Nonce             string   `json:"nonce,omitempty"`
		PreferredUsername string   `json:"preferred_username"`
		Groups            []string `json:"groups"`
	}
	c := claims{
		Claims: jwt.Claims{
			Issuer: m.srv.URL, Subject: "user-1", Audience: jwt.Audience{testClientID},
			Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now()),
		},
		Nonce: nonce, PreferredUsername: "alice", Groups: m.groups,
	}
	if m.badNonce {
		c.Nonce = "wrong"
	}
	s, err := jwt.Signed(m.signer).Claims(c).Serialize()
	if err != nil {
		m.t.Fatal(err)
	}
	return s
}

func (m *mockIDP) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.down {
		http.Error(w, "boom", 500)
		return
	}
	invalid := func() {
		writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
	}
	resp := map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "rt-1"}

	switch r.Form.Get("grant_type") {
	case "authorization_code":
		info, ok := m.codes[r.Form.Get("code")]
		delete(m.codes, r.Form.Get("code"))
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != info.challenge {
			invalid()
			return
		}
		resp["id_token"] = m.mint(info.nonce)
	case "refresh_token":
		if m.disabled || r.Form.Get("refresh_token") != "rt-1" {
			invalid()
			return
		}
		resp["id_token"] = m.mint("")
	default:
		invalid()
		return
	}
	writeJSON(w, 200, resp)
}

type oidcEnv struct {
	t       *testing.T
	idp     *mockIDP
	h       http.Handler
	auth    *Authenticator
	svc     *Service
	owners  *OwnerStore
	dir     string
	browser *http.Client
}

func newOIDCEnv(t *testing.T) *oidcEnv {
	t.Helper()
	idp := newMockIDP(t)
	dir := t.TempDir()
	cfg, err := LoadConfig(env(map[string]string{
		"VPN_OIDC_ISSUER":        idp.srv.URL,
		"VPN_OIDC_CLIENT_ID":     testClientID,
		"VPN_OIDC_REDIRECT_URL":  testOrigin + "/auth/callback",
		"VPN_MAX_PEERS_PER_USER": "2",
		"VPN_PEER_TTL":           "1h",
		"VPN_NET4":               "10.10.30.0/28",
		"VPN_DATA_DIR":           dir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	fb := newFakeBackend()
	if err := fb.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := NewService(dir, cfg.Pool, fb)
	owners, err := NewOwnerStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthenticator(context.Background(), *cfg.OIDC, &api{cfg: cfg, svc: svc, backend: fb}, owners)
	if err != nil {
		t.Fatal(err)
	}
	return &oidcEnv{
		t: t, idp: idp, auth: auth, svc: svc, owners: owners, dir: dir,
		h: newHandler(cfg, svc, fb, auth),
		browser: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.MaxAge >= 0 && c.Value != "" {
			return c
		}
	}
	return nil
}

// startLogin runs /auth/login and the provider's authorize step, returning the
// callback request a browser would make.
func (e *oidcEnv) startLogin() *http.Request {
	e.t.Helper()
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/login", nil))
	if rec.Code != http.StatusFound {
		e.t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	state := cookieNamed(rec, stateCookie)
	if state == nil {
		e.t.Fatal("login did not set the state cookie")
	}
	resp, err := e.browser.Get(rec.Header().Get("Location"))
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		e.t.Fatalf("authorize: %d", resp.StatusCode)
	}
	cb, _ := url.Parse(resp.Header.Get("Location"))
	req := httptest.NewRequest("GET", cb.RequestURI(), nil)
	req.AddCookie(state)
	return req
}

func (e *oidcEnv) signIn() (*httptest.ResponseRecorder, *http.Cookie) {
	e.t.Helper()
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, e.startLogin())
	return rec, cookieNamed(rec, sessionCookie)
}

func (e *oidcEnv) mustSignIn() *http.Cookie {
	e.t.Helper()
	rec, c := e.signIn()
	if rec.Code != http.StatusSeeOther || c == nil {
		e.t.Fatalf("sign-in failed: %d %s", rec.Code, rec.Body)
	}
	return c
}

func (e *oidcEnv) do(method, path string, c *http.Cookie, headers map[string]string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if c != nil {
		req.AddCookie(c)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

var sameOrigin = map[string]string{"Origin": testOrigin}

func TestOIDCSelfService(t *testing.T) {
	e := newOIDCEnv(t)
	c := e.mustSignIn()

	if rec := e.do("GET", "/api/v1/me", c, nil, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"alice"`) {
		t.Fatalf("me: %d %s", rec.Code, rec.Body)
	}

	var first createPeerResponse
	rec := e.do("POST", "/api/v1/me/peers", c, sameOrigin, `{"name":"laptop"}`)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	_ = json.NewDecoder(rec.Body).Decode(&first)
	if first.Peer.Owner != "user-1" || first.Peer.ExpiresAt == nil {
		t.Fatalf("owner/expiry not set: %+v", first.Peer)
	}
	if d := time.Until(*first.Peer.ExpiresAt); d < 50*time.Minute || d > 61*time.Minute {
		t.Errorf("expiry %v away, want about 1h", d)
	}
	if rec := e.do("POST", "/api/v1/me/peers", c, sameOrigin, `{"name":"phone"}`); rec.Code != 201 {
		t.Fatalf("second peer: %d", rec.Code)
	}
	if rec := e.do("POST", "/api/v1/me/peers", c, sameOrigin, `{"name":"third"}`); rec.Code != http.StatusConflict {
		t.Fatalf("limit not enforced: %d", rec.Code)
	}
	if got := len(e.svc.ListOwned("user-1")); got != 2 {
		t.Fatalf("owned peers: %d", got)
	}

	other, _, err := e.svc.Create(context.Background(), "theirs", "", CreateOptions{Owner: "someone-else"})
	if err != nil {
		t.Fatal(err)
	}
	if rec := e.do("DELETE", "/api/v1/me/peers/"+other.ID, c, sameOrigin, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("deleting another user's peer: %d", rec.Code)
	}
	if rec := e.do("DELETE", "/api/v1/me/peers/"+first.Peer.ID, c, sameOrigin, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete own: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/v1/me/peers", c, nil, ""); strings.Contains(rec.Body.String(), "someone-else") || rec.Code != 200 {
		t.Fatalf("list leaks other users: %s", rec.Body)
	}

	if rec := e.do("POST", "/auth/logout", c, sameOrigin, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/v1/me", c, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session survived logout: %d", rec.Code)
	}
}

func TestOIDCRejectsUnauthenticatedAndCrossOrigin(t *testing.T) {
	e := newOIDCEnv(t)
	if rec := e.do("GET", "/api/v1/me", nil, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/v1/me", &http.Cookie{Name: sessionCookie, Value: "bogus"}, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("bogus cookie: %d", rec.Code)
	}

	c := e.mustSignIn()
	for name, h := range map[string]map[string]string{
		"foreign origin":  {"Origin": "https://evil.example"},
		"cross-site hint": {"Sec-Fetch-Site": "cross-site"},
	} {
		if rec := e.do("POST", "/api/v1/me/peers", c, h, `{"name":"x"}`); rec.Code != http.StatusForbidden {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if got := len(e.svc.ListOwned("user-1")); got != 0 {
		t.Errorf("a refused request still created %d peer(s)", got)
	}
}

func TestOIDCSignInChecks(t *testing.T) {
	t.Run("not in group", func(t *testing.T) {
		e := newOIDCEnv(t)
		e.idp.groups = []string{"other"}
		rec, c := e.signIn()
		if rec.Code != http.StatusForbidden || c != nil {
			t.Fatalf("got %d, session %v", rec.Code, c)
		}
	})
	t.Run("wrong nonce", func(t *testing.T) {
		e := newOIDCEnv(t)
		e.idp.badNonce = true
		if rec, c := e.signIn(); rec.Code != http.StatusUnauthorized || c != nil {
			t.Fatalf("got %d, session %v", rec.Code, c)
		}
	})
	t.Run("state cookie missing or wrong", func(t *testing.T) {
		e := newOIDCEnv(t)
		req := e.startLogin()
		wrong := httptest.NewRequest("GET", req.URL.RequestURI(), nil)
		wrong.AddCookie(&http.Cookie{Name: stateCookie, Value: "other"})
		for _, r := range []*http.Request{httptest.NewRequest("GET", req.URL.RequestURI(), nil), wrong} {
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, r)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("got %d", rec.Code)
			}
		}
	})
	t.Run("replay", func(t *testing.T) {
		e := newOIDCEnv(t)
		req := e.startLogin()
		replay := httptest.NewRequest("GET", req.URL.RequestURI(), nil)
		replay.AddCookie(req.Cookies()[0])

		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("first use: %d", rec.Code)
		}
		rec = httptest.NewRecorder()
		e.h.ServeHTTP(rec, replay)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("replay: %d", rec.Code)
		}
	})
}

func (e *oidcEnv) peerCount() int { return len(e.svc.ListOwned("user-1")) }

func TestReconcile(t *testing.T) {
	setup := func(t *testing.T) (*oidcEnv, *http.Cookie) {
		e := newOIDCEnv(t)
		c := e.mustSignIn()
		if rec := e.do("POST", "/api/v1/me/peers", c, sameOrigin, `{"name":"laptop"}`); rec.Code != 201 {
			t.Fatalf("create: %d", rec.Code)
		}
		return e, c
	}
	revoked := func(t *testing.T, e *oidcEnv, c *http.Cookie) {
		t.Helper()
		e.auth.Reconcile(context.Background())
		if e.peerCount() != 0 {
			t.Error("peers were not revoked")
		}
		if rec := e.do("GET", "/api/v1/me", c, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("session survived revocation: %d", rec.Code)
		}
		if _, ok := e.owners.Snapshot()["user-1"]; ok {
			t.Error("owner record not removed")
		}
	}

	t.Run("healthy user keeps peers", func(t *testing.T) {
		e, _ := setup(t)
		e.auth.Reconcile(context.Background())
		if e.peerCount() != 1 {
			t.Error("peer removed for a healthy user")
		}
	})
	t.Run("deactivated user", func(t *testing.T) {
		e, c := setup(t)
		e.idp.disabled = true
		revoked(t, e, c)
	})
	t.Run("removed from group", func(t *testing.T) {
		e, c := setup(t)
		e.idp.groups = []string{"something-else"}
		revoked(t, e, c)
	})
	t.Run("provider outage keeps peers", func(t *testing.T) {
		e, _ := setup(t)
		e.idp.down = true
		e.auth.Reconcile(context.Background())
		if e.peerCount() != 1 {
			t.Error("an outage must not revoke peers")
		}
	})
	t.Run("expired peers", func(t *testing.T) {
		e, _ := setup(t)
		e.auth.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
		e.auth.Reconcile(context.Background())
		if e.peerCount() != 0 {
			t.Error("expired peer kept")
		}
	})
}

func TestOwnerStorePersistsSecurely(t *testing.T) {
	e := newOIDCEnv(t)
	e.mustSignIn()
	again, err := NewOwnerStore(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	if rec := again.Snapshot()["user-1"]; rec.Name != "alice" || rec.RefreshToken != "rt-1" {
		t.Fatalf("owner not persisted: %+v", rec)
	}
}

func TestOIDCConfigValidation(t *testing.T) {
	base := map[string]string{
		"VPN_OIDC_ISSUER":       "https://sso.example.com/application/o/vpn/",
		"VPN_OIDC_CLIENT_ID":    "id",
		"VPN_OIDC_REDIRECT_URL": "https://vpn.example.com/auth/callback",
	}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range base {
			m[a] = b
		}
		m[k] = v
		return m
	}
	if cfg, err := LoadConfig(env(base)); err != nil || cfg.OIDC == nil || cfg.OIDC.Group != "vpn-users" {
		t.Fatalf("valid config: %v %+v", err, cfg.OIDC)
	}
	for name, e := range map[string]map[string]string{
		"http issuer":     with("VPN_OIDC_ISSUER", "http://sso.example.com/"),
		"no client id":    with("VPN_OIDC_CLIENT_ID", ""),
		"http redirect":   with("VPN_OIDC_REDIRECT_URL", "http://vpn.example.com/auth/callback"),
		"wrong path":      with("VPN_OIDC_REDIRECT_URL", "https://vpn.example.com/cb"),
		"zero max peers":  with("VPN_MAX_PEERS_PER_USER", "0"),
		"negative ttl":    with("VPN_PEER_TTL", "-1h"),
		"bad ttl":         with("VPN_PEER_TTL", "soon"),
		"fast reconciler": with("VPN_RECONCILE_INTERVAL", "1s"),
	} {
		if _, err := LoadConfig(env(e)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
