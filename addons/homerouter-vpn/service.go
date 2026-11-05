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
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	PublicKey string     `json:"publicKey"`
	Address   string     `json:"address"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// Owner is the OIDC subject for self-service peers; admin-created peers have none.
	Owner     string `json:"owner,omitempty"`
	OwnerName string `json:"ownerName,omitempty"`
}

// CreateOptions carries what differs between admin and self-service creation.
type CreateOptions struct {
	Owner, OwnerName string
	TTL              time.Duration // 0 = never expires
	MaxPerOwner      int           // 0 = unlimited
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
	return writeFileAtomic(s.path, b)
}

// writeFileAtomic replaces path via a temp file; the result is readable by the owner only.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Service) List() []Peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Peer{}, s.peers...)
}

func (s *Service) ListOwned(owner string) []Peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Peer{}
	for _, p := range s.peers {
		if p.Owner == owner {
			out = append(out, p)
		}
	}
	return out
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
func (s *Service) Create(ctx context.Context, name, publicKey string, opt CreateOptions) (Peer, string, error) {
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
	owned := 0
	for _, p := range s.peers {
		if p.PublicKey == publicKey {
			return Peer{}, "", fmt.Errorf("%w: public key already registered", ErrConflict)
		}
		if opt.Owner != "" && p.Owner == opt.Owner {
			owned++
		}
		if a, err := netip.ParseAddr(p.Address); err == nil {
			used[a] = true
		}
	}
	if opt.MaxPerOwner > 0 && owned >= opt.MaxPerOwner {
		return Peer{}, "", fmt.Errorf("%w: peer limit of %d reached", ErrConflict, opt.MaxPerOwner)
	}
	addr, err := s.pool.Allocate(used)
	if err != nil {
		return Peer{}, "", fmt.Errorf("%w: %v", ErrConflict, err)
	}

	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return Peer{}, "", err
	}
	now := time.Now().UTC().Truncate(time.Second)
	peer := Peer{
		ID: hex.EncodeToString(id), Name: name, PublicKey: publicKey,
		Address: addr.String(), CreatedAt: now,
		Owner: opt.Owner, OwnerName: opt.OwnerName,
	}
	if opt.TTL > 0 {
		exp := now.Add(opt.TTL)
		peer.ExpiresAt = &exp
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
	return s.deleteOne(ctx, func(p Peer) bool { return p.ID == id })
}

// DeleteOwned removes a peer only if it belongs to owner.
func (s *Service) DeleteOwned(ctx context.Context, id, owner string) error {
	return s.deleteOne(ctx, func(p Peer) bool { return p.ID == id && p.Owner == owner })
}

func (s *Service) deleteOne(ctx context.Context, match func(Peer) bool) error {
	n, err := s.removeWhere(ctx, match)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveOwner revokes every peer of an owner.
func (s *Service) RemoveOwner(ctx context.Context, owner string) (int, error) {
	return s.removeWhere(ctx, func(p Peer) bool { return p.Owner == owner })
}

// RemoveExpired revokes peers whose expiry has passed.
func (s *Service) RemoveExpired(ctx context.Context, now time.Time) (int, error) {
	return s.removeWhere(ctx, func(p Peer) bool { return p.ExpiresAt != nil && !p.ExpiresAt.After(now) })
}

// removeWhere removes matching peers from the backend and the store. Peers
// the backend fails to remove are kept so the next run retries them.
func (s *Service) removeWhere(ctx context.Context, match func(Peer) bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kept []Peer
	var firstErr error
	removed := 0
	for _, p := range s.peers {
		if !match(p) {
			kept = append(kept, p)
			continue
		}
		if err := s.backend.RemovePeer(ctx, p.PublicKey); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			kept = append(kept, p)
			continue
		}
		removed++
	}
	if removed == 0 {
		return 0, firstErr
	}

	prev := s.peers
	s.peers = kept
	if err := s.save(); err != nil {
		s.peers = prev
		return 0, err
	}
	return removed, firstErr
}
