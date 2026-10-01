package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const passwordIterations = 310000

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

type passwordHash struct {
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Iterations int    `json:"iterations"`
}

type proxyUser struct {
	Username string       `json:"username"`
	Password passwordHash `json:"password"`
	Disabled bool         `json:"disabled"`
	Created  time.Time    `json:"created"`
}

type proxyUserView struct {
	Username string    `json:"username"`
	Disabled bool      `json:"disabled"`
	Created  time.Time `json:"created"`
}

type diskState struct {
	AdminUsername   string               `json:"admin_username"`
	AdminPassword   passwordHash         `json:"admin_password"`
	AdminMustChange bool                 `json:"admin_must_change_password"`
	ProxyUsers      map[string]proxyUser `json:"proxy_users"`
}

type userStore struct {
	mu    sync.RWMutex
	path  string
	state diskState
}

func newPasswordHash(password string) (passwordHash, error) {
	if password == "" {
		return passwordHash{}, errors.New("password must not be empty")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return passwordHash{}, fmt.Errorf("generate password salt: %w", err)
	}
	hash, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return passwordHash{}, fmt.Errorf("derive password hash: %w", err)
	}
	return passwordHash{
		Salt:       base64.RawStdEncoding.EncodeToString(salt),
		Hash:       base64.RawStdEncoding.EncodeToString(hash),
		Iterations: passwordIterations,
	}, nil
}

func verifyPassword(password string, stored passwordHash) bool {
	salt, err := base64.RawStdEncoding.DecodeString(stored.Salt)
	if err != nil || stored.Iterations < 100000 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(stored.Hash)
	if err != nil || len(want) != 32 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, stored.Iterations, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func openUserStore(path, adminUsername, bootstrapPassword string) (*userStore, error) {
	if adminUsername == "" {
		adminUsername = "admin"
	}
	store := &userStore{path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &store.state); err != nil {
			return nil, fmt.Errorf("decode proxy user store: %w", err)
		}
		if store.state.ProxyUsers == nil {
			store.state.ProxyUsers = make(map[string]proxyUser)
		}
		if err := os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("secure proxy user store: %w", err)
		}
		return store, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read proxy user store: %w", err)
	}
	if bootstrapPassword == "" {
		return nil, errors.New("PROXY_ADMIN_PASSWORD is required to initialize the admin account")
	}
	adminHash, err := newPasswordHash(bootstrapPassword)
	if err != nil {
		return nil, err
	}
	store.state = diskState{
		AdminUsername:   adminUsername,
		AdminPassword:   adminHash,
		AdminMustChange: true,
		ProxyUsers:      make(map[string]proxyUser),
	}
	if err := store.saveLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *userStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create proxy data directory: %w", err)
	}
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proxy user store: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".proxy-users-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary proxy user store: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure temporary proxy user store: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write proxy user store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync proxy user store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close proxy user store: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace proxy user store: %w", err)
	}
	return os.Chmod(s.path, 0600)
}

func (s *userStore) adminUsername() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.AdminUsername
}

func (s *userStore) adminMustChange() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.AdminMustChange
}

func (s *userStore) authenticateAdmin(username, password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return username == s.state.AdminUsername && verifyPassword(password, s.state.AdminPassword)
}

func (s *userStore) changeAdminPassword(current, next string) error {
	if len(next) < 12 {
		return errors.New("new admin password must be at least 12 characters")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !verifyPassword(current, s.state.AdminPassword) {
		return errors.New("current admin password is incorrect")
	}
	hash, err := newPasswordHash(next)
	if err != nil {
		return err
	}
	old := s.state.AdminPassword
	oldMustChange := s.state.AdminMustChange
	s.state.AdminPassword = hash
	s.state.AdminMustChange = false
	if err := s.saveLocked(); err != nil {
		s.state.AdminPassword = old
		s.state.AdminMustChange = oldMustChange
		return err
	}
	return nil
}

func (s *userStore) authenticateProxy(username, password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.state.ProxyUsers[username]
	return ok && !user.Disabled && verifyPassword(password, user.Password)
}

func (s *userStore) listProxyUsers() []proxyUserView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]proxyUserView, 0, len(s.state.ProxyUsers))
	for _, user := range s.state.ProxyUsers {
		users = append(users, proxyUserView{
			Username: user.Username,
			Disabled: user.Disabled,
			Created:  user.Created,
		})
	}
	return users
}

func (s *userStore) createProxyUser(username, password string) error {
	if !usernamePattern.MatchString(username) {
		return errors.New("username must contain only letters, numbers, dot, underscore, or hyphen")
	}
	if len(password) < 12 {
		return errors.New("proxy password must be at least 12 characters")
	}
	hash, err := newPasswordHash(password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.state.ProxyUsers[username]; exists {
		return errors.New("proxy user already exists")
	}
	s.state.ProxyUsers[username] = proxyUser{
		Username: username,
		Password: hash,
		Created:  time.Now().UTC(),
	}
	if err := s.saveLocked(); err != nil {
		delete(s.state.ProxyUsers, username)
		return err
	}
	return nil
}

func (s *userStore) setProxyUserDisabled(username string, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, exists := s.state.ProxyUsers[username]
	if !exists {
		return errors.New("proxy user not found")
	}
	old := user.Disabled
	user.Disabled = disabled
	s.state.ProxyUsers[username] = user
	if err := s.saveLocked(); err != nil {
		user.Disabled = old
		s.state.ProxyUsers[username] = user
		return err
	}
	return nil
}

func (s *userStore) deleteProxyUser(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, exists := s.state.ProxyUsers[username]
	if !exists {
		return errors.New("proxy user not found")
	}
	delete(s.state.ProxyUsers, username)
	if err := s.saveLocked(); err != nil {
		s.state.ProxyUsers[username] = user
		return err
	}
	return nil
}