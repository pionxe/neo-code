package gateway

import "testing"

func TestGatewayMetricsSnapshot(t *testing.T) {
	metrics := NewGatewayMetrics()
	metrics.IncRequests("ipc", "gateway.ping", "ok")
	metrics.IncRequests("ipc", "gateway.executeSystemTool", "ok")
	metrics.IncRequests("ipc", "gateway.listAvailableSkills", "ok")
	metrics.IncAuthFailures("ws", "unauthorized")
	metrics.IncACLDenied("http", "wake.openUrl")
	metrics.SetConnectionsActive("ws", 2)
	metrics.IncStreamDropped("queue_full")

	snapshot := metrics.Snapshot()
	if snapshot["gateway_requests_total"]["ipc|gateway.ping|ok"] != 1 {
		t.Fatalf("requests snapshot mismatch: %#v", snapshot["gateway_requests_total"])
	}
	if snapshot["gateway_requests_total"]["ipc|gateway.executesystemtool|ok"] != 1 {
		t.Fatalf("executeSystemTool snapshot mismatch: %#v", snapshot["gateway_requests_total"])
	}
	if snapshot["gateway_requests_total"]["ipc|gateway.listavailableskills|ok"] != 1 {
		t.Fatalf("listAvailableSkills snapshot mismatch: %#v", snapshot["gateway_requests_total"])
	}
	if snapshot["gateway_auth_failures_total"]["ws|unauthorized"] != 1 {
		t.Fatalf("auth failures snapshot mismatch: %#v", snapshot["gateway_auth_failures_total"])
	}
	if snapshot["gateway_acl_denied_total"]["http|wake.openurl"] != 1 {
		t.Fatalf("acl denied snapshot mismatch: %#v", snapshot["gateway_acl_denied_total"])
	}
	if snapshot["gateway_connections_active"]["ws"] != 2 {
		t.Fatalf("connections gauge snapshot mismatch: %#v", snapshot["gateway_connections_active"])
	}
	if snapshot["gateway_stream_dropped_total"]["queue_full"] != 1 {
		t.Fatalf("stream dropped snapshot mismatch: %#v", snapshot["gateway_stream_dropped_total"])
	}
}

func TestGatewayMetricsNilReceiverAndLabelNormalization(t *testing.T) {
	var metrics *GatewayMetrics
	if metrics.Registry() != nil {
		t.Fatal("nil metrics registry should be nil")
	}
	if snapshot := metrics.Snapshot(); len(snapshot) != 0 {
		t.Fatalf("nil metrics snapshot = %#v, want empty", snapshot)
	}
	metrics.IncRequests("", "", "")
	metrics.IncAuthFailures("", "")
	metrics.IncACLDenied("", "")
	metrics.SetConnectionsActive("", 1)
	metrics.IncStreamDropped("")

	realMetrics := NewGatewayMetrics()
	realMetrics.IncRequests(" IPC ", " gateway.ping ", " ")
	realMetrics.IncAuthFailures(" HTTP ", " ")
	realMetrics.IncACLDenied(" WS ", " ")
	realMetrics.SetConnectionsActive(" ", 3)
	realMetrics.IncStreamDropped(" ")

	snapshot := realMetrics.Snapshot()
	if snapshot["gateway_requests_total"]["ipc|gateway.ping|unknown"] != 1 {
		t.Fatalf("normalized request labels mismatch: %#v", snapshot["gateway_requests_total"])
	}
	if snapshot["gateway_auth_failures_total"]["http|unknown"] != 1 {
		t.Fatalf("normalized auth labels mismatch: %#v", snapshot["gateway_auth_failures_total"])
	}
	if snapshot["gateway_acl_denied_total"]["ws|unknown_method"] != 1 {
		t.Fatalf("normalized acl labels mismatch: %#v", snapshot["gateway_acl_denied_total"])
	}
	if snapshot["gateway_connections_active"]["unknown"] != 3 {
		t.Fatalf("normalized connection labels mismatch: %#v", snapshot["gateway_connections_active"])
	}
	if snapshot["gateway_stream_dropped_total"]["unknown"] != 1 {
		t.Fatalf("normalized dropped labels mismatch: %#v", snapshot["gateway_stream_dropped_total"])
	}
}

func TestGatewayMetricsSnapshotMapRecreateBranches(t *testing.T) {
	metrics := NewGatewayMetrics()
	delete(metrics.snapshot, "gateway_requests_total")
	delete(metrics.snapshot, "gateway_connections_active")
	metrics.IncRequests("ipc", "gateway.ping", "ok")
	metrics.SetConnectionsActive("ipc", 1)
	snapshot := metrics.Snapshot()
	if snapshot["gateway_requests_total"]["ipc|gateway.ping|ok"] != 1 {
		t.Fatalf("requests snapshot mismatch: %#v", snapshot["gateway_requests_total"])
	}
	if snapshot["gateway_connections_active"]["ipc"] != 1 {
		t.Fatalf("connections snapshot mismatch: %#v", snapshot["gateway_connections_active"])
	}
}

func TestGatewayMetricsUnknownMethodCollapsed(t *testing.T) {
	metrics := NewGatewayMetrics()
	metrics.IncRequests("http", "random.method.from.user", "ok")
	metrics.IncACLDenied("ws", "random.method.from.user")

	snapshot := metrics.Snapshot()
	if snapshot["gateway_requests_total"]["http|unknown_method|ok"] != 1 {
		t.Fatalf("requests snapshot mismatch: %#v", snapshot["gateway_requests_total"])
	}
	if snapshot["gateway_acl_denied_total"]["ws|unknown_method"] != 1 {
		t.Fatalf("acl denied snapshot mismatch: %#v", snapshot["gateway_acl_denied_total"])
	}
}
