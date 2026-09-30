package analysis

import (
	"os"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestRetainedSnapshotDensityAnalysis(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ANALYSIS_RUN")
	if root == "" {
		t.Skip("requires sealed native density product run")
	}
	b, err := experiment.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	curves, err := SnapshotDensity(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(curves) != 18 {
		t.Fatalf("got %d curves, want twelve memory and six diagnostic clock curves", len(curves))
	}
	for _, c := range curves {
		if len(c.Points) != 5 || len(c.Marginal) != 4 || c.LogLogSlope != nil {
			t.Fatalf("bad curve: %+v", c)
		}
		for _, p := range c.Points {
			if p.Attempted != 2 || p.Launches != 2 || p.Median == nil || p.Low != nil || p.High != nil || p.Status != "insufficient_launches" {
				t.Fatalf("inner groups inflated replication: %+v", p)
			}
		}
		for _, m := range c.Marginal {
			if m.PairedBlocks != 2 || m.Median == nil || m.Low != nil || m.Status != "insufficient_blocks" {
				t.Fatalf("bad pairing: %+v", m)
			}
		}
	}
	// One denied reading invalidates its entire launch for that stage, never
	// merely omits that process or averages the remaining convenient groups.
	deniedTrial := -1
	for i := range b.Trials {
		if b.Trials[i].Block >= 0 && b.Trials[i].Status == "ok" {
			deniedTrial = i
			break
		}
	}
	reading := &b.Trials[deniedTrial].SnapshotDensity.Groups[0].Records[1].Readings[2]
	saved := *reading
	reading.SmapsRollup = nil
	reading.SmapsStatus = "permission_denied"
	reading.SmapsReason = "analysis coverage test"
	denied, err := SnapshotDensity(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range denied {
		if c.Runtime != b.Trials[deniedTrial].Runtime || c.Measurement.Phase != "idle" {
			continue
		}
		for _, p := range c.Points {
			if p.Workload == b.Trials[deniedTrial].Workload && (p.Launches != 1 || p.Outcomes["incomplete_smaps_coverage"] != 1) {
				t.Fatalf("denial silently dropped: %+v", p)
			}
		}
	}
	*reading = saved
	// Malformed successful evidence is an error, not a silently omitted point.
	for i := range b.Trials {
		if b.Trials[i].Block >= 0 && b.Trials[i].Status == "ok" {
			b.Trials[i].SnapshotDensity.Groups[0].Proof.ChildrenReaped = false
			break
		}
	}
	if _, err := SnapshotDensity(b); err == nil {
		t.Fatal("accepted incomplete cleanup proof")
	}
}

func TestDensityDiagnosticTime(t *testing.T) {
	a, b := int64(0), int64(3)
	p := protocol.SnapshotDensityProof{ProvisionElapsedNS: &a, Children: []protocol.SnapshotDensityChild{{TouchElapsedNS: &a, ExecuteElapsedNS: &b}, {TouchElapsedNS: &b, ExecuteElapsedNS: &a}}}
	for metric, want := range map[string]float64{"snapshot.provision.elapsed": 0, "snapshot.child_touch.mean_elapsed": 1.5, "snapshot.child_execution.mean_elapsed": 1.5} {
		got, err := densityDiagnosticTime(p, metric)
		if err != nil || got != want {
			t.Fatalf("%s: %g, %v", metric, got, err)
		}
	}
	for _, bad := range []int64{-1, 9007199254740992} {
		p.ProvisionElapsedNS = &bad
		if _, err := densityDiagnosticTime(p, "snapshot.provision.elapsed"); err == nil {
			t.Fatal("accepted bad clock")
		}
	}
	p.ProvisionElapsedNS = nil
	if _, err := densityDiagnosticTime(p, "snapshot.provision.elapsed"); err == nil {
		t.Fatal("accepted missing clock")
	}
	p.Children = nil
	if _, err := densityDiagnosticTime(p, "snapshot.child_execution.mean_elapsed"); err == nil {
		t.Fatal("accepted no children")
	}
	max := int64(9007199254740991)
	for range 32 {
		p.Children = append(p.Children, protocol.SnapshotDensityChild{TouchElapsedNS: &max})
	}
	if got, err := densityDiagnosticTime(p, "snapshot.child_touch.mean_elapsed"); err != nil || got != float64(max) {
		t.Fatalf("overflowed group reduction: %g, %v", got, err)
	}
	p.Children[0].TouchElapsedNS = nil
	if _, err := densityDiagnosticTime(p, "snapshot.child_touch.mean_elapsed"); err == nil {
		t.Fatal("dropped missing child's clock")
	}
	if _, err := densityDiagnosticTime(p, "unknown"); err == nil {
		t.Fatal("accepted unknown metric")
	}
}
