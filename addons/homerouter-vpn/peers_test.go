package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef"

func newTestServer(t *testing.T, dir string) (http.Handler, *fakeBackend) {
	t.Helper()
	cfg, err := LoadConfig(env(map[string]string{
		"VPN_API_TOKEN":   testToken,
		"VPN_NET4":        "10.10.30.0/29",
		"VPN_DNS":         "10.10.10.1",
		"VPN_ALLOWED_IPS": "10.10.10.0/24, 10.10.30.0/29",
		"VPN_DATA_DIR":    dir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	fb := newFakeBackend()
	if err := fb.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := NewService(dir, cfg.Pool, fb)
	return newHandler(cfg, svc, fb, nil), fb
}

func call(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPIRequiresToken(t *testing.T) {
	h, _ := newTestServer(t, t.TempDir())
	for _, tok := range []string{"", "wrong", testToken + "x"} {
		if rec := call(h, "GET", "/api/v1/peers", tok, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d", tok, rec.Code)
		}
	}
	if rec := call(h, "GET", "/api/v1/peers", testToken, ""); rec.Code != http.StatusOK {
		t.Fatalf("valid token: %d", rec.Code)
	}
}

func TestAPIDisabledWithoutToken(t *testing.T) {
	cfg, _ := LoadConfig(env(nil))
	h := newHandler(cfg, nil, nil, nil)
	if rec := call(h, "GET", "/api/v1/peers", "anything", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestPeerLifecycle(t *testing.T) {
	dir := t.TempDir()
	h, fb := newTestServer(t, dir)

	rec := call(h, "POST", "/api/v1/peers", testToken, `{"name":"laptop"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created createPeerResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Peer.Address != "10.10.30.2" {
		t.Errorf("first peer address: %s", created.Peer.Address)
	}
	for _, want := range []string{"PrivateKey = ", "Address = 10.10.30.2/32", "DNS = 10.10.10.1",
		"PublicKey = " + fb.PublicKey(), "AllowedIPs = 10.10.10.0/24, 10.10.30.0/29"} {
		if !strings.Contains(created.Config, want) {
			t.Errorf("config missing %q:\n%s", want, created.Config)
		}
	}
	if strings.Contains(created.Config, "<your private key>") {
		t.Error("generated peer should carry its private key")
	}
	if _, ok := fb.peers[created.Peer.PublicKey]; !ok {
		t.Error("backend did not receive the peer")
	}

	if b, _ := os.ReadFile(filepath.Join(dir, "peers.json")); strings.Contains(string(b), "PrivateKey") {
		t.Error("private key must not be persisted")
	}

	if rec := call(h, "DELETE", "/api/v1/peers/"+created.Peer.ID, testToken, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if len(fb.peers) != 0 {
		t.Error("backend still has the peer")
	}
	if rec := call(h, "DELETE", "/api/v1/peers/"+created.Peer.ID, testToken, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: %d", rec.Code)
	}
}

func TestCreateWithClientKeyAndValidation(t *testing.T) {
	h, _ := newTestServer(t, t.TempDir())
	_, pub, _ := GenerateKeyPair()

	body, _ := json.Marshal(map[string]string{"name": "phone", "publicKey": pub})
	rec := call(h, "POST", "/api/v1/peers", testToken, string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "your private key") {
		t.Error("client-supplied key must not yield a private key")
	}
	if rec := call(h, "POST", "/api/v1/peers", testToken, string(body)); rec.Code != http.StatusConflict {
		t.Errorf("duplicate key: %d", rec.Code)
	}

	bad := map[string]string{
		"empty name": `{"name":""}`,
		"bad key":    `{"name":"x","publicKey":"nope"}`,
		"unknown":    `{"name":"x","extra":1}`,
		"not json":   `name=x`,
		"long name":  `{"name":"` + strings.Repeat("a", 65) + `"}`,
		"flag-like":  `{"name":"x","publicKey":"-----------------------------------------="}`,
		"control":    `{"name":"a\u0000b"}`,
		"oversized":  `{"name":"` + strings.Repeat("a", 5000) + `"}`,
		"empty body": ``,
		"array":      `[]`,
		"wrong type": `{"name":1}`,
		"null":       `null`,
	}
	for name, b := range bad {
		if rec := call(h, "POST", "/api/v1/peers", testToken, b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d", name, rec.Code)
		}
	}
}

func TestPoolExhaustionAndReuse(t *testing.T) {
	h, _ := newTestServer(t, t.TempDir())
	// /29: .1 server, .2-.6 peers, .7 broadcast.
	var ids []string
	for i := 0; i < 5; i++ {
		rec := call(h, "POST", "/api/v1/peers", testToken, `{"name":"p"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("peer %d: %d %s", i, rec.Code, rec.Body)
		}
		var c createPeerResponse
		_ = json.NewDecoder(rec.Body).Decode(&c)
		ids = append(ids, c.Peer.ID)
	}
	if rec := call(h, "POST", "/api/v1/peers", testToken, `{"name":"p"}`); rec.Code != http.StatusConflict {
		t.Fatalf("exhausted pool: %d", rec.Code)
	}
	call(h, "DELETE", "/api/v1/peers/"+ids[0], testToken, "")
	rec := call(h, "POST", "/api/v1/peers", testToken, `{"name":"again"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"address":"10.10.30.2"`) {
		t.Fatalf("freed address not reused: %d %s", rec.Code, rec.Body)
	}
}

func TestRestoreReappliesPeers(t *testing.T) {
	dir := t.TempDir()
	h, _ := newTestServer(t, dir)
	call(h, "POST", "/api/v1/peers", testToken, `{"name":"keep"}`)

	pool, _ := NewPool("10.10.30.0/29")
	fb := newFakeBackend()
	svc := NewService(dir, pool, fb)
	if err := svc.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(svc.List()) != 1 || len(fb.peers) != 1 {
		t.Fatalf("restore: %d peers, %d in backend", len(svc.List()), len(fb.peers))
	}
}

func TestWGBackendCommands(t *testing.T) {
	var calls []string
	w := newWGBackend("wg0", 51820, netip.MustParsePrefix("10.10.30.1/24"), t.TempDir())
	w.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "ip" && args[0] == "link" && args[1] == "show" {
			return nil, os.ErrNotExist // interface missing
		}
		return nil, nil
	}
	ctx := context.Background()
	if err := w.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if w.PublicKey() == "" {
		t.Error("server public key not set")
	}
	_, pub, _ := GenerateKeyPair()
	if err := w.AddPeer(ctx, pub, netip.MustParseAddr("10.10.30.2")); err != nil {
		t.Fatal(err)
	}
	if err := w.RemovePeer(ctx, pub); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(calls, "\n")
	for _, want := range []string{
		"ip link add dev wg0 type wireguard",
		"ip address replace 10.10.30.1/24 dev wg0",
		"wg set wg0 listen-port 51820 private-key " + w.keyPath,
		"ip link set up dev wg0",
		"wg set wg0 peer " + pub + " allowed-ips 10.10.30.2/32",
		"wg set wg0 peer " + pub + " remove",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing command %q in:\n%s", want, got)
		}
	}
	fi, err := os.Stat(w.keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("server key file mode: %v", fi.Mode())
	}
}

func TestKeysAndPool(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := PublicFromPrivate(priv); got != pub {
		t.Error("derived public key differs")
	}
	if !ValidPublicKey(pub) || ValidPublicKey("short") || ValidPublicKey(strings.Repeat("A", 44)) {
		t.Error("ValidPublicKey")
	}
	if _, err := NewPool("10.0.0.0/30"); err == nil {
		t.Error("/30 should be rejected")
	}
	if _, err := NewPool("fd00::/64"); err == nil {
		t.Error("IPv6 should be rejected")
	}
	p, _ := NewPool("10.10.30.5/24")
	if p.ServerAddr().String() != "10.10.30.1/24" {
		t.Errorf("server addr %s", p.ServerAddr())
	}
}

func TestConfigVPNValidation(t *testing.T) {
	bad := map[string]map[string]string{
		"short token":    {"VPN_API_TOKEN": "short"},
		"backend":        {"VPN_BACKEND": "other"},
		"iface":          {"VPN_IF": "wg0; rm"},
		"port":           {"VPN_PORT": "70000"},
		"subnet":         {"VPN_NET4": "nonsense"},
		"endpoint":       {"VPN_ENDPOINT": "no-port"},
		"wg no endpoint": {"VPN_BACKEND": "wg"},
		"dns":            {"VPN_DNS": "dns.example"},
		"allowed":        {"VPN_ALLOWED_IPS": "10.0.0.1"},
	}
	for name, e := range bad {
		if _, err := LoadConfig(env(e)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := LoadConfig(env(map[string]string{"VPN_BACKEND": "wg", "VPN_ENDPOINT": "vpn.example.com:51820"})); err != nil {
		t.Errorf("valid wg config: %v", err)
	}
}
