package controlplane

import "testing"

func TestDecideTurnBudgetAccurateBranches(t *testing.T) {
	t.Parallel()

	baseEstimate := TurnBudgetEstimate{
		ID: TurnBudgetID{
			AttemptSeq:  1,
			RequestHash: "hash-1",
		},
		EstimatedInputTokens: 120,
		EstimateSource:       "provider",
		GatePolicy:           TurnBudgetGatePolicyGateable,
	}

	within := DecideTurnBudget(baseEstimate, 120, 0)
	if within.Action != TurnBudgetActionAllow {
		t.Fatalf("within.Action = %q", within.Action)
	}
	if within.Reason != BudgetDecisionReasonWithinBudget {
		t.Fatalf("within.Reason = %q", within.Reason)
	}
	if within.EstimateGatePolicy != TurnBudgetGatePolicyGateable {
		t.Fatalf("within.EstimateGatePolicy = %q, want %q", within.EstimateGatePolicy, TurnBudgetGatePolicyGateable)
	}

	firstExceed := DecideTurnBudget(baseEstimate, 100, 0)
	if firstExceed.Action != TurnBudgetActionCompact {
		t.Fatalf("firstExceed.Action = %q", firstExceed.Action)
	}
	if firstExceed.Reason != BudgetDecisionReasonExceedsBudgetFirstTime {
		t.Fatalf("firstExceed.Reason = %q", firstExceed.Reason)
	}
	if firstExceed.EstimateGatePolicy != TurnBudgetGatePolicyGateable {
		t.Fatalf("firstExceed.EstimateGatePolicy = %q, want %q", firstExceed.EstimateGatePolicy, TurnBudgetGatePolicyGateable)
	}

	afterCompact := DecideTurnBudget(baseEstimate, 100, 1)
	if afterCompact.Action != TurnBudgetActionStop {
		t.Fatalf("afterCompact.Action = %q", afterCompact.Action)
	}
	if afterCompact.Reason != BudgetDecisionReasonExceedsBudgetAfterCompactStop {
		t.Fatalf("afterCompact.Reason = %q", afterCompact.Reason)
	}
	if afterCompact.EstimateGatePolicy != TurnBudgetGatePolicyGateable {
		t.Fatalf("afterCompact.EstimateGatePolicy = %q, want %q", afterCompact.EstimateGatePolicy, TurnBudgetGatePolicyGateable)
	}
}

func TestDecideTurnBudgetAdvisoryBranches(t *testing.T) {
	t.Parallel()

	estimate := TurnBudgetEstimate{
		ID: TurnBudgetID{
			AttemptSeq:  2,
			RequestHash: "hash-2",
		},
		EstimatedInputTokens: 200,
		EstimateSource:       "local",
		GatePolicy:           TurnBudgetGatePolicyAdvisory,
	}

	firstExceed := DecideTurnBudget(estimate, 100, 0)
	if firstExceed.Action != TurnBudgetActionCompact {
		t.Fatalf("firstExceed.Action = %q", firstExceed.Action)
	}
	if firstExceed.Reason != BudgetDecisionReasonExceedsBudgetFirstTime {
		t.Fatalf("firstExceed.Reason = %q", firstExceed.Reason)
	}
	if firstExceed.EstimateGatePolicy != TurnBudgetGatePolicyAdvisory {
		t.Fatalf("firstExceed.EstimateGatePolicy = %q, want %q", firstExceed.EstimateGatePolicy, TurnBudgetGatePolicyAdvisory)
	}

	afterCompact := DecideTurnBudget(estimate, 100, 1)
	if afterCompact.Action != TurnBudgetActionAllow {
		t.Fatalf("afterCompact.Action = %q", afterCompact.Action)
	}
	if afterCompact.Reason != BudgetDecisionReasonExceedsBudgetAfterCompactAllowAdvisory {
		t.Fatalf("afterCompact.Reason = %q", afterCompact.Reason)
	}
	if afterCompact.EstimateGatePolicy != TurnBudgetGatePolicyAdvisory {
		t.Fatalf("afterCompact.EstimateGatePolicy = %q, want %q", afterCompact.EstimateGatePolicy, TurnBudgetGatePolicyAdvisory)
	}
}
