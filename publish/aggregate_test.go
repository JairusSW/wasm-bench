package publish

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func aggregateBundle(t *testing.T, failed bool) (string, analysis.AggregateSet) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	m := experiment.Manifest{ID: "aggregate-fixture", Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "timing", Scenarios: []string{"compile"}, Launches: 3}, Runtimes: []experiment.Runtime{{ID: "baseline"}, {ID: "candidate"}}, Workloads: []protocol.Workload{{ID: "required", Family: "algorithms"}}}}
	if err := experiment.WriteJSON(filepath.Join(root, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	for block := 0; block < 3; block++ {
		for i, id := range []string{"baseline", "candidate"} {
			tr := experiment.Trial{ID: fmt.Sprintf("t%d-%d", block, i), Runtime: id, Workload: "required", Scenario: "compile", Profile: "timing", Block: block, Status: "ok", Samples: []protocol.Sample{{ElapsedNS: 100 * int64(i+1), Operations: 1, Verified: true}}}
			if failed && id == "candidate" {
				tr.Status = "unsupported"
				tr.Samples = nil
			}
			if err := experiment.WriteJSON(filepath.Join(root, "trials", tr.ID+".json"), tr); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := experiment.Seal(root); err != nil {
		t.Fatal(err)
	}
	b, err := experiment.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	set, err := analysis.NewAggregateSet(b, "</script><script>alert(1)</script>", "compile")
	if err != nil {
		t.Fatal(err)
	}
	return root, set
}

func TestAggregateReportPortableEvidence(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			root, set := aggregateBundle(t, failed)
			out := filepath.Join(t.TempDir(), "report")
			before, _ := experiment.DigestFile(filepath.Join(root, "checksums.json"))
			if err := AggregateReport(root, set, "baseline", "candidate", out); err != nil {
				t.Fatal(err)
			}
			for _, dir := range []string{root, out, filepath.Join(out, "raw")} {
				if err := experiment.Verify(dir); err != nil {
					t.Fatal(err)
				}
			}
			var data aggregateDataset
			if err := experiment.ReadJSON(filepath.Join(out, "data.json"), &data); err != nil {
				t.Fatal(err)
			}
			if data.SourceChecksumsSHA256 != before || data.Analysis.Set.ID != set.ID {
				t.Fatal(data)
			}
			if failed {
				if data.Analysis.Overall.Ratio != nil || data.Analysis.Coverage[0].CandidateOutcomes["unsupported"] != 3 {
					t.Fatal(data)
				}
			} else if data.Analysis.Overall.Ratio == nil || math.Abs(*data.Analysis.Overall.Ratio-2) > 1e-12 {
				t.Fatal(data)
			}
			html, err := os.ReadFile(filepath.Join(out, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(html), "</script><script>alert") || !strings.Contains(string(html), `\u003c/script\u003e`) {
				t.Fatal("unsafe embedded identity")
			}
			for _, link := range []string{"data.json", "set.json", "raw/manifest.json", "trials.json", "raw/checksums.json", "checksums.json"} {
				if !strings.Contains(string(html), `href="`+link+`"`) {
					t.Fatal("missing evidence link", link)
				}
				if _, err := os.Stat(filepath.Join(out, link)); err != nil {
					t.Fatal(err)
				}
			}
			if err := AggregateReport(root, set, "baseline", "candidate", out); err == nil {
				t.Fatal("overwrote report")
			}
			if err := experiment.Verify(out); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAggregateReportRejectsNestedAndInvalidInput(t *testing.T) {
	root, set := aggregateBundle(t, false)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{root, alias} {
		out := filepath.Join(base, "new", "report")
		if err := AggregateReport(root, set, "baseline", "candidate", out); err == nil || !strings.Contains(err.Error(), "inside") {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Dir(out)); !os.IsNotExist(err) {
			t.Fatal("input modified")
		}
	}
	out := filepath.Join(t.TempDir(), "report")
	if err := AggregateReport(root, set, "baseline", "missing", out); err == nil {
		t.Fatal("missing runtime accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("invalid report created")
	}
}
