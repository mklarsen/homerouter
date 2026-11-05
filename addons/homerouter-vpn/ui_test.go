package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIServedWithStrictCSP(t *testing.T) {
	cfg, _ := LoadConfig(env(nil))
	h := newHandler(cfg, nil, nil, nil)

	for path, wantType := range map[string]string{
		"/":             "text/html",
		"/ui/app.js":    "javascript",
		"/ui/style.css": "text/css",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), wantType) {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Header().Get("Content-Type"))
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("%s: weak CSP %q", path, csp)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown path: %d", rec.Code)
	}
}

// The page handles keys and user-controlled names, so it must never build
// markup from strings, and the HTML must stay free of inline code.
func TestUIAvoidsInjectionSinks(t *testing.T) {
	js, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function"} {
		if strings.Contains(string(js), sink) {
			t.Errorf("app.js uses %s", sink)
		}
	}
	html, _ := webFS.ReadFile("web/index.html")
	for _, inline := range []string{"onclick=", "onerror=", "<style", "javascript:"} {
		if strings.Contains(string(html), inline) {
			t.Errorf("index.html contains %q", inline)
		}
	}
	if strings.Contains(string(html), "<script>") {
		t.Error("index.html has an inline script")
	}
}

func TestQRSVGAndCreateResponse(t *testing.T) {
	svg, err := qrSVG("[Interface]\nPrivateKey = abc\n")
	if err != nil || !strings.HasPrefix(svg, "<svg ") || !strings.Contains(svg, "<path") {
		t.Fatalf("qr: %v %.40s", err, svg)
	}

	h, _ := newTestServer(t, t.TempDir())
	rec := call(h, "POST", "/api/v1/peers", testToken, `{"name":"laptop"}`)
	var resp createPeerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil || !strings.HasPrefix(resp.QRSVG, "<svg ") {
		t.Fatalf("create response lacks a QR code: %v", err)
	}
}
