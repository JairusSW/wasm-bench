package publish

import (
	"reflect"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestSamplingGroupUsesSourceTrialsNotReportOrBlocks(t *testing.T) {
	d := siteFixture()
	original, err := siteSamplingGroup([]experiment.Bundle{d.Bundle}, d.Bundle.Manifest.ID, "engine", "fixture/a", "steady", "timing", nil)
	if err != nil || original == nil || original.TrialCount != 1 {
		t.Fatal(original, err)
	}
	// A report's derived fields/versions do not participate in source identity.
	d.AnalysisVersion = "different-report-analysis"
	repeated, err := siteSamplingGroup([]experiment.Bundle{d.Bundle}, d.Bundle.Manifest.ID, "engine", "fixture/a", "steady", "timing", nil)
	if err != nil || !reflect.DeepEqual(original, repeated) {
		t.Fatal("report metadata changed sampling source", err)
	}
	other := d.Bundle
	other.Manifest.ID = "different-run-same-block"
	changed, err := siteSamplingGroup([]experiment.Bundle{other}, other.Manifest.ID, "engine", "fixture/a", "steady", "timing", nil)
	if err != nil || changed.ID == original.ID {
		t.Fatal("block number equated separate runs", err)
	}
	other = d.Bundle
	other.Trials = append([]experiment.Trial{}, d.Bundle.Trials...)
	other.Trials[0].Reason = "changed source observation"
	changed, err = siteSamplingGroup([]experiment.Bundle{other}, other.Manifest.ID, "engine", "fixture/a", "steady", "timing", nil)
	if err != nil || changed.ID == original.ID {
		t.Fatal("changed trial reused sampling group", err)
	}
	if group, err := siteSamplingGroup([]experiment.Bundle{d.Bundle}, "missing", "engine", "fixture/a", "steady", "timing", nil); err != nil || group != nil {
		t.Fatal("missing source was inferred", err)
	}
	if _, err := siteSamplingGroup([]experiment.Bundle{d.Bundle}, d.Bundle.Manifest.ID, "engine", "fixture/a", "steady", "timing", map[string]bool{"absent": true}); err == nil {
		t.Fatal("absent selected trial accepted")
	}
}
