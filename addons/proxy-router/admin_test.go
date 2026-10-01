package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
	service := &server{users: users, sessions: newSessionStore(), apiToken: "service-api-token"}
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
	read := request(http.MethodGet, "/api/v1/users/alice", "", "service-api-token")
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), "password") {
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
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"active_users":1`) {
		t.Fatalf("API status did not report user overview: %d %s", status.Code, status.Body.String())
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
