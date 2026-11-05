package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrInvalid  = errors.New("invalid request")
	ErrNotFound = errors.New("peer not found")
	ErrConflict = errors.New("conflict")
)

type Peer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey string    `json:"publicKey"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"createdAt"`
}

// Service owns the peer list: it allocates addresses, applies changes to the
// backend and persists them.
type Service struct {
	mu      sync.Mutex
	path    string
	pool    *Pool
	backend Backend
	peers   []Peer
}

func NewService(dataDir string, pool *Pool, backend Backend) *Service {
	return &Service{path: filepath.Join(dataDir, "peers.json"), pool: pool, backend: backend}
}

// Restore loads stored peers and re-applies them to the backend.
func (s *Service) Restore(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &s.peers); err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	for _, p := range s.peers {
		addr, err := netip.ParseAddr(p.Address)
		if err != nil {
			return fmt.Errorf("peer %s: %w", p.ID, err)
		}
		if err := s.backend.AddPeer(ctx, p.PublicKey, addr); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) save() error {
	b, err := json.MarshalIndent(s.peers, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Service) List() []Peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Peer{}, s.peers...)
}

func validName(n string) bool {
	if n == "" || len([]rune(n)) > 64 {
		return false
	}
	for _, r := range n {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Create adds a peer. When publicKey is empty a key pair is generated and the
// private key is returned once; it is never stored.
func (s *Service) Create(ctx context.Context, name, publicKey string) (Peer, string, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return Peer{}, "", fmt.Errorf("%w: name must be 1-64 characters without control characters", ErrInvalid)
	}

	var priv string
	if publicKey == "" {
		var err error
		if priv, publicKey, err = GenerateKeyPair(); err != nil {
			return Peer{}, "", err
		}
	} else if !ValidPublicKey(publicKey) {
		return Peer{}, "", fmt.Errorf("%w: publicKey must be a 32-byte base64 key", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	used := map[netip.Addr]bool{}
	for _, p := range s.peers {
		if p.PublicKey == publicKey {
			return Peer{}, "", fmt.Errorf("%w: public key already registered", ErrConflict)
		}
		if a, err := netip.ParseAddr(p.Address); err == nil {
			used[a] = true
		}
	}
	addr, err := s.pool.Allocate(used)
	if err != nil {
		return Peer{}, "", fmt.Errorf("%w: %v", ErrConflict, err)
	}

	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return Peer{}, "", err
	}
	peer := Peer{
		ID: hex.EncodeToString(id), Name: name, PublicKey: publicKey,
		Address: addr.String(), CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := s.backend.AddPeer(ctx, publicKey, addr); err != nil {
		return Peer{}, "", err
	}
	s.peers = append(s.peers, peer)
	if err := s.save(); err != nil {
		s.peers = s.peers[:len(s.peers)-1]
		_ = s.backend.RemovePeer(ctx, publicKey)
		return Peer{}, "", err
	}
	return peer, priv, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, p := range s.peers {
		if p.ID != id {
			continue
		}
		if err := s.backend.RemovePeer(ctx, p.PublicKey); err != nil {
			return err
		}
		next := append(append([]Peer{}, s.peers[:i]...), s.peers[i+1:]...)
		prev := s.peers
		s.peers = next
		if err := s.save(); err != nil {
			s.peers = prev
			return err
		}
		return nil
	}
	return ErrNotFound
}
