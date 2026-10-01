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