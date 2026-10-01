package main

import (
	"context"
	"sync"
)

type proxyActivity struct {
	mu          sync.Mutex
	nextID      uint64
	connections map[string]map[uint64]context.CancelFunc
}

func newProxyActivity() *proxyActivity {
	return &proxyActivity{connections: make(map[string]map[uint64]context.CancelFunc)}
}

func (a *proxyActivity) begin(username string, cancel context.CancelFunc) func() {
	if a == nil || username == "" {
		return func() {}
	}
	a.mu.Lock()
	a.nextID++
	connectionID := a.nextID
	if a.connections[username] == nil {
		a.connections[username] = make(map[uint64]context.CancelFunc)
	}
	a.connections[username][connectionID] = cancel
	a.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			delete(a.connections[username], connectionID)
			if len(a.connections[username]) == 0 {
				delete(a.connections, username)
			}
		})
	}
}

func (a *proxyActivity) disconnect(username string) int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	callbacks := make([]context.CancelFunc, 0, len(a.connections[username]))
	for _, cancel := range a.connections[username] {
		callbacks = append(callbacks, cancel)
	}
	a.mu.Unlock()
	for _, cancel := range callbacks {
		cancel()
	}
	return len(callbacks)
}

func (a *proxyActivity) count(username string) int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.connections[username])
}

func (a *proxyActivity) summary() (activeUsers, activeConnections int) {
	if a == nil {
		return 0, 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, connections := range a.connections {
		count := len(connections)
		if count > 0 {
			activeUsers++
			activeConnections += count
		}
	}
	return activeUsers, activeConnections
}
