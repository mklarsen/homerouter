package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8081" || cfg.Brand.Name != "VPN" || cfg.Brand.Color != "#2563eb" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadConfigRejectsBadBrand(t *testing.T) {
	cases := map[string]map[string]string{
		"color":        {"VPN_BRAND_COLOR": "red; background:url(x)"},
		"short color":  {"VPN_BRAND_COLOR": "#fff"},
		"logo scheme":  {"VPN_BRAND_LOGO_URL": "javascript:alert(1)"},
		"logo http":    {"VPN_BRAND_LOGO_URL": "http://example.com/l.png"},
		"name control": {"VPN_BRAND_NAME": "bad\nname"},
		"name length":  {"VPN_BRAND_NAME": strings.Repeat("a", 65)},
	}
	for name, e := range cases {
		if _, err := LoadConfig(env(e)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoadConfigAcceptsBrand(t *testing.T) {
	cfg, err := LoadConfig(env(map[string]string{
		"VPN_BRAND_NAME":     "Example Access",
		"VPN_BRAND_COLOR":    "#00AA55",
		"VPN_BRAND_LOGO_URL": "https://example.com/logo.svg",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Brand.Name != "Example Access" || cfg.Brand.LogoURL != "https://example.com/logo.svg" {
		t.Fatalf("unexpected brand: %+v", cfg.Brand)
	}
}

func TestHandlers(t *testing.T) {
	cfg, _ := LoadConfig(env(map[string]string{"VPN_BRAND_NAME": "Example"}))
	h := newHandler(cfg, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/brand", nil))
	var b Brand
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil || b.Name != "Example" {
		t.Fatalf("brand: %v %+v", err, b)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz: %d", rec.Code)
	}
}
