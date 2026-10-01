package main

import (
	"net"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestProxyAuthenticationChallenge(t *testing.T) {
	store, err := openUserStore(filepath.Join(t.TempDir(), "users.json"), "admin", "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.createProxyUser("alice", "a-long-proxy-password-123"); err != nil {
		t.Fatal(err)
	}
	service := &server{users: store}

	request := httptest.NewRequest("CONNECT", "https://example.com:443", nil)
	request.Header.Set("Proxy-Authorization", "Basic YWxpY2U6YS1sb25nLXByb3h5LXBhc3N3b3JkLTEyMw==")
	response := httptest.NewRecorder()
	if !service.authenticateProxyRequest(response, request) {
		t.Fatalf("valid proxy user rejected with status %d", response.Code)
	}

	request = httptest.NewRequest("CONNECT", "https://example.com:443", nil)
	request.Header.Set("Proxy-Authorization", "Basic YWxpY2U6d3Jvbmc=")
	response = httptest.NewRecorder()
	if service.authenticateProxyRequest(response, request) {
		t.Fatal("invalid proxy password accepted")
	}
	if response.Code != 407 || response.Header().Get("Proxy-Authenticate") == "" {
		t.Fatalf("expected HTTP 407 auth challenge, got %d", response.Code)
	}
}

func TestPublicAddressPolicy(t *testing.T) {
	blockedAddresses := []string{
		"127.0.0.1", "10.10.10.1", "169.254.169.254", "100.64.0.1",
		"192.0.2.1", "192.88.99.1", "240.0.0.1", "::1", "fd00::1",
		"::ffff:127.0.0.1", "2001:2::1", "2001:db8::1", "2001:10::1",
		"2001:20::1", "2002::1", "3fff::1",
		"64:ff9b:1::1", "5f00::1", "fec0::1",
	}
	for _, address := range blockedAddresses {
		if isPublicIP(net.ParseIP(address)) {
			t.Errorf("private/reserved address allowed: %s", address)
		}
	}
	for _, address := range []string{"1.1.1.1", "2606:4700:4700::1111", "2001:4860:4860::8888", "::ffff:1.1.1.1"} {
		if !isPublicIP(net.ParseIP(address)) {
			t.Errorf("public address blocked: %s", address)
		}
	}
}
