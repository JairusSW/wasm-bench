package sourcebuild

import (
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestSourceAdmissionEmptyHostPinsSurviveJSONRoundTrip(t *testing.T) {
	built := Result{Lock: Lock{Recipe: Recipe{Workload: protocol.Workload{ID: "w"}}}, ArtifactSHA256: "artifact"}
	w := emittedWorkload(built, "")
	want := experiment.Runtime{ID: "r", HostFiles: map[string]string{}}
	got := want
	got.HostFiles = nil
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "correctness_only", Lock: experiment.Lock{Workloads: []protocol.Workload{w}, Runtimes: []experiment.Runtime{got}}}, Trials: []experiment.Trial{{Runtime: "r", Status: "ok", Samples: []protocol.Sample{{Verified: true}}}}}
	if err := verifyAdmission(b, built, []experiment.Runtime{want}); err != nil {
		t.Fatal("empty host pins changed identity after JSON", err)
	}
	for _, mode := range []string{"missing", "changed", "extra"} {
		want.HostFiles = map[string]string{"host-library": "expected"}
		switch mode {
		case "missing":
			got.HostFiles = nil
		case "changed":
			got.HostFiles = map[string]string{"host-library": "changed"}
		case "extra":
			got.HostFiles = map[string]string{"host-library": "expected", "extra": "pin"}
		}
		b.Manifest.Lock.Runtimes = []experiment.Runtime{got}
		if err := verifyAdmission(b, built, []experiment.Runtime{want}); err == nil {
			t.Fatal("relaxed nonempty host pins", mode)
		}
	}
}
