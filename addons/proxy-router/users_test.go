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

func TestProxyUserReadAndUpdate(t *testing.T) {
	store, err := openUserStore(filepath.Join(t.TempDir(), "users.json"), "admin", "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.createProxyUser("mkl", "before-password-123"); err != nil {
		t.Fatal(err)
	}
	view, err := store.getProxyUser("mkl")
	if err != nil || view.Username != "mkl" || view.Disabled {
		t.Fatalf("unexpected proxy user view: %#v, %v", view, err)
	}

	newPassword := "after-password-456"
	disabled := true
	if err := store.updateProxyUser("mkl", &newPassword, &disabled); err != nil {
		t.Fatal(err)
	}
	if store.authenticateProxy("mkl", "before-password-123") || store.authenticateProxy("mkl", newPassword) {
		t.Fatal("password update or disable state was not applied")
	}
	view, err = store.getProxyUser("mkl")
	if err != nil || !view.Disabled {
		t.Fatalf("updated user state was not returned: %#v, %v", view, err)
	}
	active, disabledCount := store.proxyUserCounts()
	if active != 0 || disabledCount != 1 {
		t.Fatalf("unexpected user counts: active=%d disabled=%d", active, disabledCount)
	}
}

func TestProxyLogLevelPersistsAndValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	store, err := openUserStore(path, "admin", "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if store.logLevel() != "INFO" {
		t.Fatalf("default log level = %q, want INFO", store.logLevel())
	}
	if err := store.setLogLevel("debug"); err != nil {
		t.Fatal(err)
	}
	if store.logLevel() != "DEBUG" || activeProxyLogLevelName() != "INFO" {
		t.Fatalf("persisted setting or runtime level was unexpectedly coupled: store=%q runtime=%q", store.logLevel(), activeProxyLogLevelName())
	}
	if setActiveProxyLogLevel(store.logLevel()) != true || activeProxyLogLevelName() != "DEBUG" {
		t.Fatal("runtime log level did not accept persisted setting")
	}
	if err := store.setLogLevel("TRACE"); err == nil {
		t.Fatal("unsupported log level accepted")
	}
	reloaded, err := openUserStore(path, "admin", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.logLevel() != "DEBUG" {
		t.Fatalf("reloaded log level = %q, want DEBUG", reloaded.logLevel())
	}
	setActiveProxyLogLevel(defaultProxyLogLevel)
}
