//go:build linux

package agent

import "testing"

func TestIRQAffinityNativeReadOnly(t *testing.T) {
	p := ProbeIRQAffinity("0")
	if err := ValidateIRQAffinityProbe(p); err != nil {
		t.Fatal(err)
	}
	if len(p.Facts) == 0 {
		t.Fatal("native Linux probe retained no facts")
	}
	t.Logf("status=%s reason=%s facts=%d", p.Status, p.Reason, len(p.Facts))
}
