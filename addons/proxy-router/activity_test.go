package main

import "testing"

func TestProxyActivityTracksActiveUsersAndConnections(t *testing.T) {
	activity := newProxyActivity()
	aliceFirst := activity.begin("alice")
	aliceSecond := activity.begin("alice")
	bob := activity.begin("bob")

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
