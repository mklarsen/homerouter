package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type ownerRecord struct {
	Name string `json:"name"`
	// RefreshToken lets the reconciler re-check the user with the identity
	// provider. It is a secret: the file is 0600 and lives next to server.key.
	RefreshToken string `json:"refreshToken,omitempty"`
}

// OwnerStore remembers the OIDC users that own peers.
type OwnerStore struct {
	mu   sync.Mutex
	path string
	m    map[string]ownerRecord
}

func NewOwnerStore(dataDir string) (*OwnerStore, error) {
	s := &OwnerStore{path: filepath.Join(dataDir, "owners.json"), m: map[string]ownerRecord{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.m); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *OwnerStore) saveLocked() error {
	b, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, b)
}

// Set records a user. An empty token keeps any token already stored.
func (s *OwnerStore) Set(sub, name, refreshToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.m[sub]
	rec.Name = name
	if refreshToken != "" {
		rec.RefreshToken = refreshToken
	}
	s.m[sub] = rec
	return s.saveLocked()
}

func (s *OwnerStore) Delete(sub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, sub)
	return s.saveLocked()
}

func (s *OwnerStore) Snapshot() map[string]ownerRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]ownerRecord, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}
