package main

import (
	"path/filepath"
	"testing"
)

func TestAdminBootstrapRequiresPasswordRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	store, err := openUserStore(path, "admin", "admin/admin")
	if err != nil {
		t.Fatal(err)
	}
	if !store.adminMustChange() {
		t.Fatal("bootstrap admin must be required to rotate password")
	}
	if !store.authenticateAdmin("admin", "admin/admin") {
		t.Fatal("bootstrap credentials were not accepted")
	}
	if err := store.changeAdminPassword("admin/admin", "a-long-admin-password-123"); err != nil {
		t.Fatal(err)
	}
	if store.adminMustChange() || store.authenticateAdmin("admin", "admin/admin") {
		t.Fatal("bootstrap password was not rotated")
	}
	reloaded, err := openUserStore(path, "admin", "ignored-bootstrap-password")
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.authenticateAdmin("admin", "a-long-admin-password-123") {
		t.Fatal("rotated password did not persist")
	}
}

func TestProxyUserLifecycle(t *testing.T) {
	store, err := openUserStore(filepath.Join(t.TempDir(), "users.json"), "admin", "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	password := "a-long-proxy-password-123"
	if err := store.createProxyUser("alice", password); err != nil {
		t.Fatal(err)
	}
	if !store.authenticateProxy("alice", password) || store.authenticateProxy("alice", "incorrect") {
		t.Fatal("proxy password verification failed")
	}
	if err := store.setProxyUserDisabled("alice", true); err != nil {
		t.Fatal(err)
	}
	if store.authenticateProxy("alice", password) {
		t.Fatal("disabled proxy user authenticated")
	}
	if err := store.deleteProxyUser("alice"); err != nil {
		t.Fatal(err)
	}
	if store.authenticateProxy("alice", password) {
		t.Fatal("deleted proxy user authenticated")
	}
}
