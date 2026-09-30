package publish

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestRetainedSnapshotDensityFootprints(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ANALYSIS_RUN")
	if root == "" {
		t.Skip("requires native sealed density evidence")
	}
	b, err := experiment.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	views, err := snapshotDensityViews(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 20 {
		t.Fatalf("got %d measured views", len(views))
	}
	readings := 0
	for _, v := range views {
		if v.RetainedBoundaries != 8 || v.Status != "ok" {
			t.Fatalf("bad view: %+v", v)
		}
		for _, row := range v.Rows {
			readings++
			if row.GroupIndex != row.SampleIndex || row.RawRecordIndex < 0 || row.RawRecordIndex > 3 || row.RSSBytes == "" || row.Process.StartTimeTicks == "" {
				t.Fatalf("bad raw coordinates: %+v", row)
			}
			if (row.Role == "restored") != (row.ChildIndex != nil) {
				t.Fatalf("bad role: %+v", row)
			}
			if row.ChildIndex != nil && *row.ChildIndex != row.RawReadingIndex-2 {
				t.Fatal("child index drift")
			}
		}
	}
	if readings != 1448 {
		t.Fatalf("got %d readings", readings)
	}
	index := -1
	for i := range b.Trials {
		if b.Trials[i].Block >= 0 && b.Trials[i].Status == "ok" {
			index = i
			break
		}
	}
	trial := &b.Trials[index]
	for gi := range trial.SnapshotDensity.Groups {
		for ri := range trial.SnapshotDensity.Groups[gi].Records {
			for pi := range trial.SnapshotDensity.Groups[gi].Records[ri].Readings {
				r := &trial.SnapshotDensity.Groups[gi].Records[ri].Readings[pi]
				r.StartNS += 9007199254740993
				r.EndNS += 9007199254740993
			}
		}
	}
	r := &trial.SnapshotDensity.Groups[0].Records[1].Readings[2]
	r.SmapsRollup = nil
	r.SmapsStatus = "permission_denied"
	r.SmapsReason = "fixture denial"
	views, err = snapshotDensityViews(b)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(views)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"start_ns":"9007`) {
		t.Fatal("collector brackets lost decimal string encoding")
	}
	for _, v := range views {
		if v.Trial == trial.ID {
			row := v.Rows[4]
			if row.SmapsStatus != "permission_denied" || row.PSSBytes != nil || row.PrivateBytes != nil || row.RSSBytes == "" {
				t.Fatalf("missing treated as zero: %+v", row)
			}
		}
	}
	trial.Status = "error"
	trial.Reason = "injected incomplete proof"
	views, err = snapshotDensityViews(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Trial == trial.ID && (len(v.Rows) != 0 || v.RetainedBoundaries != 8 || v.RawTrial == "") {
			t.Fatal("failure promoted readings")
		}
	}
	trial.Status = "ok"
	trial.SnapshotDensity.Groups[0].Proof.ChildrenReaped = false
	if _, err = snapshotDensityViews(b); err == nil {
		t.Fatal("accepted incomplete proof")
	}
}
