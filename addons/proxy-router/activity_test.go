package main

import "testing"

func TestProxyActivityTracksActiveUsersAndConnections(t *testing.T) {
	activity := newProxyActivity()
	aliceFirst := activity.begin("alice", func() {})
	aliceSecond := activity.begin("alice", func() {})
	bob := activity.begin("bob", func() {})

	activeUsers, activeConnections := activity.summary()
	if activeUsers != 2 || activeConnections != 3 {
		t.Fatalf("unexpected active summary: users=%d connections=%d", activeUsers, activeConnections)
	}
	if activity.count("alice") != 2 || activity.count("unknown") != 0 {
		t.Fatal("per-user active connection count is incorrect")
	}

	aliceFirst()
	aliceFirst()
	if activity.count("alice") != 1 {
		t.Fatal("activity cleanup was not idempotent")
	}
	aliceSecond()
	bob()
	activeUsers, activeConnections = activity.summary()
	if activeUsers != 0 || activeConnections != 0 {
		t.Fatalf("closed connections still counted: users=%d connections=%d", activeUsers, activeConnections)
	}
}

func TestProxyActivityDisconnectCancelsAllUserConnections(t *testing.T) {
	activity := newProxyActivity()
	cancelled := 0
	activity.begin("alice", func() { cancelled++ })
	activity.begin("alice", func() { cancelled++ })
	activity.begin("bob", func() { cancelled++ })

	if closed := activity.disconnect("alice"); closed != 2 || cancelled != 2 {
		t.Fatalf("disconnect did not cancel both alice connections: closed=%d cancelled=%d", closed, cancelled)
	}
	if activity.count("alice") != 2 {
		t.Fatal("cancelled connections should remain counted until their handlers exit")
	}
	if closed := activity.disconnect("unknown"); closed != 0 {
		t.Fatalf("unknown user had connections closed: %d", closed)
	}
}
