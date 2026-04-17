package gateway

import "testing"

func TestResolvedBuildInfoTrimsValues(t *testing.T) {
	originalVersion := GatewayVersion
	originalCommit := GatewayCommit
	originalBuildTime := GatewayBuildTime
	t.Cleanup(func() {
		GatewayVersion = originalVersion
		GatewayCommit = originalCommit
		GatewayBuildTime = originalBuildTime
	})

	GatewayVersion = " v1.2.3 "
	GatewayCommit = " abc123 "
	GatewayBuildTime = " 2026-04-17T00:00:00Z "

	info := ResolvedBuildInfo()
	if info["version"] != "v1.2.3" {
		t.Fatalf("version = %q, want %q", info["version"], "v1.2.3")
	}
	if info["commit"] != "abc123" {
		t.Fatalf("commit = %q, want %q", info["commit"], "abc123")
	}
	if info["build_time"] != "2026-04-17T00:00:00Z" {
		t.Fatalf("build_time = %q, want %q", info["build_time"], "2026-04-17T00:00:00Z")
	}
}
