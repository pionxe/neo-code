package internalcompact

import (
	"strings"
	"testing"
)

func TestSummarySectionsReturnsCopyInDeclaredOrder(t *testing.T) {
	t.Parallel()

	got := SummarySections()
	want := []string{
		SectionDone,
		SectionInProgress,
		SectionDecisions,
		SectionCodeChanges,
		SectionConstraints,
	}

	if len(got) != len(want) {
		t.Fatalf("SummarySections() length = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("SummarySections()[%d] = %q, want %q", index, got[index], want[index])
		}
	}

	got[0] = "mutated"
	again := SummarySections()
	if again[0] != SectionDone {
		t.Fatalf("SummarySections() should return a defensive copy, got %q", again[0])
	}
}

func TestFormatTemplateUsesSummaryMarkerAndAllSections(t *testing.T) {
	t.Parallel()

	got := FormatTemplate()

	if !strings.HasPrefix(got, SummaryMarker+"\n") {
		t.Fatalf("FormatTemplate() should start with summary marker, got %q", got)
	}

	for _, section := range SummarySections() {
		if !strings.Contains(got, section+":\n- ...") {
			t.Fatalf("FormatTemplate() missing section %q, got %q", section, got)
		}
	}

	if strings.Count(got, "\n\n") != len(SummarySections())-1 {
		t.Fatalf("FormatTemplate() paragraph separation count = %d, want %d", strings.Count(got, "\n\n"), len(SummarySections())-1)
	}
}
