package mcp

import (
	"context"
	"testing"
)

func TestDefaultExposureFilterFiltersByStatusAndPolicy(t *testing.T) {
	t.Parallel()

	filter := NewExposureFilter(ExposureFilterConfig{
		Allowlist: []string{"docs", "search.live"},
		Denylist:  []string{"docs.secret"},
	})

	snapshots := []ServerSnapshot{
		{
			ServerID: "docs",
			Status:   ServerStatusReady,
			Tools: []ToolDescriptor{
				{Name: "search"},
				{Name: "secret"},
			},
		},
		{
			ServerID: "search",
			Status:   ServerStatusReady,
			Tools: []ToolDescriptor{
				{Name: "live"},
				{Name: "private"},
			},
		},
		{
			ServerID: "offline",
			Status:   ServerStatusOffline,
			Tools: []ToolDescriptor{
				{Name: "down"},
			},
		},
	}

	filtered, decisions, err := filter.Filter(context.Background(), snapshots, ExposureFilterInput{Query: "golang"})
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 visible servers, got %d", len(filtered))
	}
	if len(filtered[0].Tools) != 1 || filtered[0].Tools[0].Name != "search" {
		t.Fatalf("expected docs.search only, got %+v", filtered[0].Tools)
	}
	if len(filtered[1].Tools) != 1 || filtered[1].Tools[0].Name != "live" {
		t.Fatalf("expected search.live only, got %+v", filtered[1].Tools)
	}

	got := map[string]ExposureFilterReason{}
	for _, decision := range decisions {
		got[decision.ToolFullName] = decision.Reason
		if decision.ToolFullName == "mcp.docs.search" && !decision.Allowed {
			t.Fatalf("expected docs.search allowed")
		}
	}
	if got["mcp.docs.secret"] != ExposureFilterReasonPolicyDeny {
		t.Fatalf("expected docs.secret denied by policy, got %q", got["mcp.docs.secret"])
	}
	if got["mcp.search.private"] != ExposureFilterReasonAllowlistMiss {
		t.Fatalf("expected search.private allowlist miss, got %q", got["mcp.search.private"])
	}
	if got["mcp.offline.down"] != ExposureFilterReasonOffline {
		t.Fatalf("expected offline.down offline, got %q", got["mcp.offline.down"])
	}
}

func TestDefaultExposureFilterFiltersByAgentRule(t *testing.T) {
	t.Parallel()

	filter := NewExposureFilter(ExposureFilterConfig{
		Agents: []AgentExposureRule{
			{Agent: "planner", Allowlist: []string{"docs"}},
			{Agent: "coder", Allowlist: []string{"docs.write"}},
		},
	})
	snapshots := []ServerSnapshot{
		{
			ServerID: "docs",
			Status:   ServerStatusReady,
			Tools: []ToolDescriptor{
				{Name: "read"},
				{Name: "write"},
			},
		},
	}

	plannerFiltered, plannerDecisions, err := filter.Filter(context.Background(), snapshots, ExposureFilterInput{Agent: "planner"})
	if err != nil {
		t.Fatalf("planner Filter() error = %v", err)
	}
	if len(plannerFiltered) != 1 || len(plannerFiltered[0].Tools) != 2 {
		t.Fatalf("expected planner to see whole docs server, got %+v", plannerFiltered)
	}

	coderFiltered, coderDecisions, err := filter.Filter(context.Background(), snapshots, ExposureFilterInput{Agent: "CoDeR"})
	if err != nil {
		t.Fatalf("coder Filter() error = %v", err)
	}
	if len(coderFiltered) != 1 || len(coderFiltered[0].Tools) != 1 || coderFiltered[0].Tools[0].Name != "write" {
		t.Fatalf("expected coder to see docs.write only, got %+v", coderFiltered)
	}

	unknownFiltered, unknownDecisions, err := filter.Filter(context.Background(), snapshots, ExposureFilterInput{Agent: "reviewer"})
	if err != nil {
		t.Fatalf("unknown Filter() error = %v", err)
	}
	if len(unknownFiltered) != 0 {
		t.Fatalf("expected unknown agent to see nothing, got %+v", unknownFiltered)
	}

	if !hasDecisionReason(plannerDecisions, "mcp.docs.read", "") || !hasDecisionReason(plannerDecisions, "mcp.docs.write", "") {
		t.Fatalf("expected planner decisions for both tools, got %+v", plannerDecisions)
	}
	if !hasDecisionReason(coderDecisions, "mcp.docs.read", ExposureFilterReasonAgentMismatch) {
		t.Fatalf("expected coder read mismatch, got %+v", coderDecisions)
	}
	if !hasDecisionReason(unknownDecisions, "mcp.docs.write", ExposureFilterReasonAgentMismatch) {
		t.Fatalf("expected unknown agent mismatch, got %+v", unknownDecisions)
	}
}

func TestDefaultExposureFilterHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	filter := NewExposureFilter(ExposureFilterConfig{})
	if _, _, err := filter.Filter(ctx, []ServerSnapshot{{ServerID: "docs"}}, ExposureFilterInput{}); err == nil {
		t.Fatalf("expected canceled context error")
	}
}

func hasDecisionReason(decisions []ExposureDecision, fullName string, reason ExposureFilterReason) bool {
	for _, decision := range decisions {
		if decision.ToolFullName != fullName {
			continue
		}
		if reason == "" {
			return decision.Allowed
		}
		return !decision.Allowed && decision.Reason == reason
	}
	return false
}
