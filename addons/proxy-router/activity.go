package main

import "sync"

type proxyActivity struct {
	mu          sync.Mutex
	connections map[string]int
}

func newProxyActivity() *proxyActivity {
	return &proxyActivity{connections: make(map[string]int)}
}

func (a *proxyActivity) begin(username string) func() {
	if a == nil || username == "" {
		return func() {}
	}
	a.mu.Lock()
	a.connections[username]++
	a.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.connections[username] <= 1 {
				delete(a.connections, username)
				return
			}
			a.connections[username]--
		})
	}
}

func (a *proxyActivity) count(username string) int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connections[username]
}

func (a *proxyActivity) summary() (activeUsers, activeConnections int) {
	if a == nil {
		return 0, 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, count := range a.connections {
		if count > 0 {
			activeUsers++
			activeConnections += count
		}
	}
	return activeUsers, activeConnections
}
