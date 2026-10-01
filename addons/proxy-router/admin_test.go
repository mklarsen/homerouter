package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVPNAdminLoginRotationAndProxyUserAPI(t *testing.T) {
	users, err := openUserStore(filepath.Join(t.TempDir(), "users.json"), "admin", "admin/admin")
	if err != nil {
		t.Fatal(err)
	}
	service := &server{users: users, sessions: newSessionStore()}

	pageRequest := httptest.NewRequest(http.MethodGet, "/vpnadm", nil)
	pageRequest.Host = "vpn.homerouter.io"
	page := httptest.NewRecorder()
	service.ServeHTTP(page, pageRequest)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "VPN ADMINISTRATION") {
		t.Fatalf("admin page not served: status %d", page.Code)
	}

	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin/admin"})
	login := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(loginBody))
	login.Host = "vpn.homerouter.io"
	login.Header.Set("Origin", "https://vpn.homerouter.io")
	loginResponse := httptest.NewRecorder()
	service.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("admin login failed: status %d, body %s", loginResponse.Code, loginResponse.Body.String())
	}
	var loginResult map[string]any
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &loginResult); err != nil {
		t.Fatal(err)
	}
	if loginResult["must_change_password"] != true {
		t.Fatal("bootstrap login did not require password change")
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("admin session cookie missing secure attributes")
	}

	listUsers := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	listUsers.Host = "vpn.homerouter.io"
	listUsers.AddCookie(cookies[0])
	blocked := httptest.NewRecorder()
	service.ServeHTTP(blocked, listUsers)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("proxy-user API available before admin password rotation: %d", blocked.Code)
	}

	passwordBody, _ := json.Marshal(map[string]string{"current_password": "admin/admin", "new_password": "a-long-admin-password-123"})
	changePassword := httptest.NewRequest(http.MethodPost, "/api/v1/admin/password", bytes.NewReader(passwordBody))
	changePassword.Host = "vpn.homerouter.io"
	changePassword.Header.Set("Origin", "https://vpn.homerouter.io")
	changePassword.AddCookie(cookies[0])
	changed := httptest.NewRecorder()
	service.ServeHTTP(changed, changePassword)
	if changed.Code != http.StatusOK {
		t.Fatalf("admin password change failed: %d %s", changed.Code, changed.Body.String())
	}

	createBody, _ := json.Marshal(map[string]string{"username": "alice", "password": "a-long-proxy-password-123"})
	createUser := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(createBody))
	createUser.Host = "vpn.homerouter.io"
	createUser.Header.Set("Origin", "https://vpn.homerouter.io")
	createUser.AddCookie(cookies[0])
	created := httptest.NewRecorder()
	service.ServeHTTP(created, createUser)
	if created.Code != http.StatusCreated {
		t.Fatalf("proxy-user create failed: %d %s", created.Code, created.Body.String())
	}
	if !users.authenticateProxy("alice", "a-long-proxy-password-123") {
		t.Fatal("created proxy credentials do not authenticate")
	}

	listUsers = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	listUsers.Host = "vpn.homerouter.io"
	listUsers.AddCookie(cookies[0])
	listed := httptest.NewRecorder()
	service.ServeHTTP(listed, listUsers)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "password") {
		t.Fatalf("user API leaked credentials or failed: %d %s", listed.Code, listed.Body.String())
	}
}

func TestProxyUserCRUDWithBearerToken(t *testing.T) {
	users, err := openUserStore(filepath.Join(t.TempDir(), "users.json"), "admin", "bootstrap-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.changeAdminPassword("bootstrap-password", "rotated-admin-password-123"); err != nil {
		t.Fatal(err)
	}
	service := &server{users: users, sessions: newSessionStore(), activity: newProxyActivity(), apiToken: "service-api-token"}
	request := func(method, path, body string, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = "vpn.homerouter.io"
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		service.ServeHTTP(response, req)
		return response
	}

	if response := request(http.MethodGet, "/api/v1/users", "", "wrong-token"); response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid service token accepted: %d", response.Code)
	}
	created := request(http.MethodPost, "/api/v1/users", `{"username":"alice","password":"first-proxy-password-123"}`, "service-api-token")
	if created.Code != http.StatusCreated {
		t.Fatalf("API create failed: %d %s", created.Code, created.Body.String())
	}
	proxyContext, cancelActivity := context.WithCancel(context.Background())
	finishActivity := service.activity.begin("alice", cancelActivity)
	defer finishActivity()
	duplicate := request(http.MethodPost, "/api/v1/users", `{"username":"alice","password":"another-proxy-password-456"}`, "service-api-token")
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate API create should return 409: %d %s", duplicate.Code, duplicate.Body.String())
	}
	read := request(http.MethodGet, "/api/v1/users/alice", "", "service-api-token")
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), "password") || !strings.Contains(read.Body.String(), `"active_connections":1`) {
		t.Fatalf("API read failed or leaked credential data: %d %s", read.Code, read.Body.String())
	}
	updated := request(http.MethodPatch, "/api/v1/users/alice", `{"password":"second-proxy-password-456","disabled":true}`, "service-api-token")
	if updated.Code != http.StatusNoContent || users.authenticateProxy("alice", "second-proxy-password-456") {
		t.Fatalf("API update failed to rotate/disable credentials: %d", updated.Code)
	}
	updated = request(http.MethodPatch, "/api/v1/users/alice", `{"disabled":false}`, "service-api-token")
	if updated.Code != http.StatusNoContent || !users.authenticateProxy("alice", "second-proxy-password-456") {
		t.Fatalf("API re-enable failed: %d", updated.Code)
	}
	status := request(http.MethodGet, "/api/v1/status", "", "service-api-token")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"active_users":1`) || !strings.Contains(status.Body.String(), `"log_level":"INFO"`) || !strings.Contains(status.Body.String(), `"active_proxy_users":1`) || !strings.Contains(status.Body.String(), `"active_proxy_connections":1`) {
		t.Fatalf("API status did not report user overview: %d %s", status.Code, status.Body.String())
	}
	loggingSettings := request(http.MethodGet, "/api/v1/settings/logging", "", "service-api-token")
	if loggingSettings.Code != http.StatusOK || !strings.Contains(loggingSettings.Body.String(), `"level":"INFO"`) {
		t.Fatalf("API did not return log settings: %d %s", loggingSettings.Code, loggingSettings.Body.String())
	}
	setLogLevel := request(http.MethodPatch, "/api/v1/settings/logging", `{"level":"DEBUG"}`, "service-api-token")
	if setLogLevel.Code != http.StatusOK || users.logLevel() != "DEBUG" || activeProxyLogLevelName() != "DEBUG" {
		t.Fatalf("API did not apply log level: %d %s", setLogLevel.Code, setLogLevel.Body.String())
	}
	invalidLogLevel := request(http.MethodPatch, "/api/v1/settings/logging", `{"level":"TRACE"}`, "service-api-token")
	if invalidLogLevel.Code != http.StatusBadRequest || users.logLevel() != "DEBUG" {
		t.Fatalf("API accepted invalid log level or changed current setting: %d %s", invalidLogLevel.Code, invalidLogLevel.Body.String())
	}
	setActiveProxyLogLevel(defaultProxyLogLevel)
	disconnect := request(http.MethodPost, "/api/v1/proxy/disconnect", `{"username":"alice","password":"second-proxy-password-456"}`, "")
	if disconnect.Code != http.StatusOK || !strings.Contains(disconnect.Body.String(), `"closed_connections":1`) {
		t.Fatalf("proxy user disconnect failed: %d %s", disconnect.Code, disconnect.Body.String())
	}
	select {
	case <-proxyContext.Done():
	default:
		t.Fatal("disconnect API did not cancel the user's active proxy context")
	}
	cancelActivity()
	finishActivity()
	status = request(http.MethodGet, "/api/v1/status", "", "service-api-token")
	if !strings.Contains(status.Body.String(), `"active_proxy_users":0`) || !strings.Contains(status.Body.String(), `"active_proxy_connections":0`) {
		t.Fatalf("closed activity still reported active: %s", status.Body.String())
	}
	deleted := request(http.MethodDelete, "/api/v1/users/alice", "", "service-api-token")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("API delete failed: %d %s", deleted.Code, deleted.Body.String())
	}
	missing := request(http.MethodGet, "/api/v1/users/alice", "", "service-api-token")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted API user still exists: %d", missing.Code)
	}
}

func TestOpenAPISpecIsServedWithoutAdminSession(t *testing.T) {
	service := &server{}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	request.Host = "vpn.homerouter.io"
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Homerouter Proxy API") {
		t.Fatalf("OpenAPI spec not served: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "yaml") {
		t.Fatalf("unexpected OpenAPI content type: %q", response.Header().Get("Content-Type"))
	}
}

func TestBrowserAPIDocsAreServedAsHTML(t *testing.T) {
	service := &server{}
	request := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	request.Host = "vpn.homerouter.io"
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Homerouter Proxy API") {
		t.Fatalf("browser API docs not served: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("unexpected browser docs content type: %q", response.Header().Get("Content-Type"))
	}
}

func TestAPITokenMinimumLength(t *testing.T) {
	if err := validateAPIToken(""); err != nil {
		t.Fatalf("empty optional API token should be accepted: %v", err)
	}
	if err := validateAPIToken("short-token"); err == nil {
		t.Fatal("short API token was accepted")
	}
	if err := validateAPIToken("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("32-character API token rejected: %v", err)
	}
}

func TestProxyDisconnectAcceptsExtensionOriginAndUserCredentials(t *testing.T) {
	users, err := openUserStore(filepath.Join(t.TempDir(), "users.json"), "admin", "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.changeAdminPassword("bootstrap", "rotated-admin-password-123"); err != nil {
		t.Fatal(err)
	}
	if err := users.createProxyUser("alice", "proxy-user-password-123"); err != nil {
		t.Fatal(err)
	}
	service := &server{users: users, sessions: newSessionStore(), activity: newProxyActivity()}
	ctx, cancel := context.WithCancel(context.Background())
	finish := service.activity.begin("alice", cancel)
	defer finish()

	extensionOrigin := "chrome-extension://abcdefghijklmnopabcdefghijklmnop"
	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/proxy/disconnect", nil)
	preflight.Host = "vpn.homerouter.io"
	preflight.Header.Set("Origin", extensionOrigin)
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	preflightResponse := httptest.NewRecorder()
	service.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent || preflightResponse.Header().Get("Access-Control-Allow-Origin") != extensionOrigin {
		t.Fatalf("extension preflight failed: %d headers=%v", preflightResponse.Code, preflightResponse.Header())
	}

	body := bytes.NewBufferString(`{"username":"alice","password":"proxy-user-password-123"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/proxy/disconnect", body)
	request.Host = "vpn.homerouter.io"
	request.Header.Set("Origin", extensionOrigin)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"closed_connections":1`) {
		t.Fatalf("extension disconnect failed: %d %s", response.Code, response.Body.String())
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("extension disconnect did not cancel the active connection")
	}
}

func TestProxyTrafficLogLevelsAndRedaction(t *testing.T) {
	var output bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
		setActiveProxyLogLevel(defaultProxyLogLevel)
	}()

	if !setActiveProxyLogLevel("INFO") {
		t.Fatal("INFO log level rejected")
	}
	logProxyEvent("auth_accepted", "alice", "192.0.2.10:50000", "CONNECT", "example.com:443", http.StatusOK, 0, 0, time.Millisecond)
	if output.Len() != 0 {
		t.Fatal("DEBUG auth event was emitted at INFO level")
	}

	logProxyEvent("http", "alice", "192.0.2.10:50000", "GET", "example.com", http.StatusOK, 0, 123, 12*time.Millisecond)
	line := output.String()
	for _, expected := range []string{"level=INFO", "user=\"alice\"", "target=\"example.com\"", "status=200", "downloaded_bytes=123"} {
		if !strings.Contains(line, expected) {
			t.Errorf("traffic log missing %q: %s", expected, line)
		}
	}
	if strings.Contains(line, "password") || strings.Contains(line, "secret") || strings.Contains(line, "?") {
		t.Fatalf("traffic log contains credential or URL detail: %s", line)
	}

	output.Reset()
	users, err := openUserStore(filepath.Join(t.TempDir(), "users.json"), "admin", "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	service := &server{users: users}
	request := httptest.NewRequest(http.MethodConnect, "https://example.com:443", nil)
	secret := "do-not-log-this-password"
	request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:"+secret)))
	response := httptest.NewRecorder()
	if _, authenticated := service.authenticateProxyUserRequest(response, request); authenticated {
		t.Fatal("invalid proxy credentials unexpectedly authenticated")
	}
	if !strings.Contains(output.String(), "level=WARNING") {
		t.Fatalf("auth failure not logged as warning: %s", output.String())
	}
	if strings.Contains(output.String(), secret) {
		t.Fatal("failed proxy password was written to the traffic log")
	}

	output.Reset()
	if !setActiveProxyLogLevel("ERROR") {
		t.Fatal("ERROR log level rejected")
	}
	logProxyEvent("http", "alice", "192.0.2.10:50000", "GET", "example.com", http.StatusOK, 0, 0, 0)
	if output.Len() != 0 {
		t.Fatal("INFO traffic was emitted at ERROR level")
	}
	logProxyEvent("http", "alice", "192.0.2.10:50000", "GET", "example.com", http.StatusBadGateway, 0, 0, 0)
	if !strings.Contains(output.String(), "level=ERROR") {
		t.Fatalf("upstream failure not logged as error: %s", output.String())
	}
}
