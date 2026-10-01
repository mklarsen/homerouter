package main

import (
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

//go:embed admin.html
var adminHTML []byte

//go:embed api-docs.html
var apiDocsHTML []byte

//go:embed openapi.yaml
var openAPISpec []byte

type server struct {
	users        *userStore
	sessions     *sessionStore
	apiToken     string
	dataHostPath string
}

type jsonError struct {
	Error string `json:"error"`
}

func validateAPIToken(token string) error {
	if token != "" && len(token) < 32 {
		return errors.New("PROXY_API_TOKEN must contain at least 32 characters")
	}
	return nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Cache-Control", "no-store")

	if !r.URL.IsAbs() && (r.URL.Path == "/vpnadm" || r.URL.Path == "/vpnadm/") {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(adminHTML)
		return
	}
	if !r.URL.IsAbs() && (r.URL.Path == "/api/docs" || r.URL.Path == "/api/docs/") {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(apiDocsHTML)
		return
	}
	if !r.URL.IsAbs() && strings.HasPrefix(r.URL.Path, "/api/v1/") {
		s.handleAdminAPI(w, r)
		return
	}
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}
	if r.URL.IsAbs() {
		s.handleForward(w, r)
		return
	}
	http.Redirect(w, r, "/vpnadm", http.StatusTemporaryRedirect)
}

func (s *server) handleAdminAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !sameOrigin(r) {
		writeAPIError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	switch path {
	case "/api/v1/openapi.yaml":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openAPISpec)
	case "/api/v1/login":
		s.handleAdminLogin(w, r)
	case "/api/v1/logout":
		s.handleAdminLogout(w, r)
	case "/api/v1/session":
		s.handleAdminSession(w, r)
	case "/api/v1/status":
		s.handleAdminStatus(w, r)
	case "/api/v1/settings/logging":
		s.handleLoggingSettings(w, r)
	case "/api/v1/admin/password":
		s.handleAdminPassword(w, r)
	case "/api/v1/users":
		s.handleProxyUsers(w, r)
	default:
		if strings.HasPrefix(path, "/api/v1/users/") {
			s.handleProxyUser(w, r, strings.TrimPrefix(path, "/api/v1/users/"))
			return
		}
		writeAPIError(w, http.StatusNotFound, "not found")
	}
}

func (s *server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !s.users.authenticateAdmin(input.Username, input.Password) {
		writeAPIError(w, http.StatusUnauthorized, "invalid admin credentials")
		return
	}
	token, err := s.sessions.create(input.Username)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "could not create admin session")
		return
	}
	setAdminCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":             input.Username,
		"must_change_password": s.users.adminMustChange(),
	})
}

func (s *server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if cookie, err := r.Cookie(adminCookieName); err == nil {
		s.sessions.delete(cookie.Value)
	}
	clearAdminCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := s.adminSession(r)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "login required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":             session.username,
		"must_change_password": s.users.adminMustChange(),
	})
}

func (s *server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	activeUsers, disabledUsers := s.users.proxyUserCounts()
	writeJSON(w, http.StatusOK, map[string]any{
		"api_base":          "/api/v1",
		"data_file":         s.users.path,
		"data_host_path":    s.dataHostPath,
		"active_users":      activeUsers,
		"disabled_users":    disabledUsers,
		"admin_must_change": s.users.adminMustChange(),
		"log_level":         s.users.logLevel(),
	})
}

func (s *server) handleLoggingSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"level":     s.users.logLevel(),
			"available": []string{"DEBUG", "INFO", "WARNING", "ERROR"},
		})
	case http.MethodPatch:
		var input struct {
			Level string `json:"level"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := s.users.setLogLevel(input.Level); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		setActiveProxyLogLevel(s.users.logLevel())
		writeJSON(w, http.StatusOK, map[string]string{"level": s.users.logLevel()})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *server) handleAdminPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := s.adminSession(r); !ok {
		writeAPIError(w, http.StatusUnauthorized, "login required")
		return
	}
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := s.users.changeAdminPassword(input.CurrentPassword, input.NewPassword); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"must_change_password": false})
}

func (s *server) handleProxyUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		users := s.users.listProxyUsers()
		sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
		writeJSON(w, http.StatusOK, users)
	case http.MethodPost:
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := s.users.createProxyUser(input.Username, input.Password); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errProxyUserAlreadyExists) {
				status = http.StatusConflict
			}
			writeAPIError(w, status, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *server) handleProxyUser(w http.ResponseWriter, r *http.Request, username string) {
	if !s.requireAdmin(w, r) {
		return
	}
	decoded, err := url.PathUnescape(username)
	if err != nil || decoded == "" || strings.Contains(decoded, "/") {
		writeAPIError(w, http.StatusBadRequest, "invalid username")
		return
	}
	switch r.Method {
	case http.MethodGet:
		user, err := s.users.getProxyUser(decoded)
		if err != nil {
			writeAPIError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodPatch:
		var input struct {
			Password *string `json:"password"`
			Disabled *bool   `json:"disabled"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := s.users.updateProxyUser(decoded, input.Password, input.Disabled); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errProxyUserNotFound) {
				status = http.StatusNotFound
			}
			writeAPIError(w, status, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.users.deleteProxyUser(decoded); err != nil {
			writeAPIError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, hasSession := s.adminSession(r)
	if !hasSession && !s.authenticateAPIToken(r) {
		writeAPIError(w, http.StatusUnauthorized, "login required")
		return false
	}
	if s.users.adminMustChange() {
		writeAPIError(w, http.StatusForbidden, "change the bootstrap admin password first")
		return false
	}
	return true
}

func (s *server) authenticateAPIToken(r *http.Request) bool {
	if s.apiToken == "" {
		return false
	}
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return false
	}
	want := sha256.Sum256([]byte(s.apiToken))
	got := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

func (s *server) adminSession(r *http.Request) (adminSession, bool) {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil {
		return adminSession{}, false
	}
	return s.sessions.valid(cookie.Value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "request must contain one JSON object")
		return false
	}
	return true
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, jsonError{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
