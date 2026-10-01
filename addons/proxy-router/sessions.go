package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const adminCookieName = "homerouter_admin"
const adminSessionLifetime = 8 * time.Hour

type adminSession struct {
	username string
	expires  time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]adminSession
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]adminSession)}
}

func (s *sessionStore) create(username string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	s.mu.Lock()
	s.sessions[token] = adminSession{username: username, expires: time.Now().Add(adminSessionLifetime)}
	s.mu.Unlock()
	return token, nil
}

func (s *sessionStore) valid(token string) (adminSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[token]
	if !ok {
		return adminSession{}, false
	}
	if time.Now().After(session.expires) {
		delete(s.sessions, token)
		return adminSession{}, false
	}
	return session, true
}

func (s *sessionStore) delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func setAdminCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(adminSessionLifetime),
		MaxAge:   int(adminSessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearAdminCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}