package experiment

import "testing"

func TestFormatOutcomeCountsIsCompactAndStable(t *testing.T) {
	got := formatOutcomeCounts(map[string]int{"ok": 7, "unsupported": 2, "failed": 1})
	want := "10 total (failed=1, ok=7, unsupported=2)"
	if got != want {
		t.Fatalf("formatOutcomeCounts() = %q, want %q", got, want)
	}
}
