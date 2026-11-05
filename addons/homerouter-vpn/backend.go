package main

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Backend applies peer changes to a WireGuard interface.
type Backend interface {
	// Ensure creates and configures the interface; it is safe to call repeatedly.
	Ensure(ctx context.Context) error
	// PublicKey is the server's public key, valid after Ensure.
	PublicKey() string
	AddPeer(ctx context.Context, publicKey string, addr netip.Addr) error
	RemovePeer(ctx context.Context, publicKey string) error
}

// fakeBackend keeps peers in memory; used for tests and local development.
type fakeBackend struct {
	mu    sync.Mutex
	pub   string
	peers map[string]netip.Addr
}

func newFakeBackend() *fakeBackend { return &fakeBackend{peers: map[string]netip.Addr{}} }

func (f *fakeBackend) Ensure(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pub == "" {
		_, pub, err := GenerateKeyPair()
		if err != nil {
			return err
		}
		f.pub = pub
	}
	return nil
}

func (f *fakeBackend) PublicKey() string { return f.pub }

func (f *fakeBackend) AddPeer(_ context.Context, key string, addr netip.Addr) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peers[key] = addr
	return nil
}

func (f *fakeBackend) RemovePeer(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.peers, key)
	return nil
}

type runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// No shell is involved; every argument is passed verbatim.
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// wgBackend drives the `wg` and `ip` tools, which need NET_ADMIN and the
// host network namespace.
type wgBackend struct {
	iface   string
	port    int
	addr    netip.Prefix
	keyPath string
	run     runner
	pub     string
}

func newWGBackend(iface string, port int, addr netip.Prefix, dataDir string) *wgBackend {
	return &wgBackend{
		iface: iface, port: port, addr: addr,
		keyPath: filepath.Join(dataDir, "server.key"),
		run:     execRunner,
	}
}

func (w *wgBackend) PublicKey() string { return w.pub }

func (w *wgBackend) cmd(ctx context.Context, name string, args ...string) error {
	if out, err := w.run(ctx, name, args...); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (w *wgBackend) Ensure(ctx context.Context) error {
	priv, err := loadOrCreateKey(w.keyPath)
	if err != nil {
		return err
	}
	if w.pub, err = PublicFromPrivate(priv); err != nil {
		return err
	}

	if _, err := w.run(ctx, "ip", "link", "show", "dev", w.iface); err != nil {
		if err := w.cmd(ctx, "ip", "link", "add", "dev", w.iface, "type", "wireguard"); err != nil {
			return err
		}
	}
	steps := [][]string{
		{"ip", "address", "replace", w.addr.String(), "dev", w.iface},
		{"wg", "set", w.iface, "listen-port", strconv.Itoa(w.port), "private-key", w.keyPath},
		{"ip", "link", "set", "up", "dev", w.iface},
	}
	for _, s := range steps {
		if err := w.cmd(ctx, s[0], s[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (w *wgBackend) AddPeer(ctx context.Context, key string, addr netip.Addr) error {
	return w.cmd(ctx, "wg", "set", w.iface, "peer", key, "allowed-ips", addr.String()+"/32")
}

func (w *wgBackend) RemovePeer(ctx context.Context, key string) error {
	return w.cmd(ctx, "wg", "set", w.iface, "peer", key, "remove")
}
