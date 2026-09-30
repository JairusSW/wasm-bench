package experiment

import (
	"context"
	"encoding/json"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func densityBundleFixture(t *testing.T) (string, Bundle) {
	root, b := snapshotEvidence(t)
	ws, err := corpus.Generate(root, "process-snapshot-density")
	if err != nil {
		t.Fatal(err)
	}
	w := ws[1]
	w.Artifact = filepath.Join("artifacts", w.SHA256+".wasm")
	b.Manifest.Lock.Workloads = []protocol.Workload{w}
	b.Manifest.Lock.Options = Options{Profile: "memory", PhaseBarriers: true, Samples: 1, Operations: 1, Scenarios: []string{protocol.SnapshotDensityScenario}, Timeout: 30 * time.Second}
	d := b.Manifest.Lock.Runtimes[0].Description
	d.Capabilities["can_inspect_linux_snapshot_density"] = true
	d.Scenarios = append(d.Scenarios, protocol.SnapshotDensityScenario)
	b.Trials = []Trial{{ID: "density", Runtime: "r", Workload: w.ID, Scenario: protocol.SnapshotDensityScenario, Profile: "memory", Status: "ok", SnapshotDensity: &SnapshotDensityTrialEvidence{Version: SnapshotDensityTrialVersion, Groups: []SnapshotDensityEvidence{densityEvidenceFixture()}}}}
	return root, b
}

func TestSnapshotDensitySealedContract(t *testing.T) {
	root, b := densityBundleFixture(t)
	if err := ValidateSnapshotDensityBundle(root, b); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Bundle){
		"missing completion": func(b *Bundle) { b.Trials[0].SnapshotDensity = nil },
		"wrong workload":     func(b *Bundle) { b.Trials[0].Workload = "foreign" },
		"wrong scenario":     func(b *Bundle) { b.Trials[0].Scenario = "density" },
		"wrong host":         func(b *Bundle) { b.Manifest.Host.OS = "darwin" },
		"wrong architecture": func(b *Bundle) { b.Manifest.Host.Arch = "amd64" },
		"timing":             func(b *Bundle) { b.Trials[0].Profile = "timing" },
		"missing barrier":    func(b *Bundle) { b.Manifest.Lock.Options.PhaseBarriers = false },
		"missing capability": func(b *Bundle) {
			b.Manifest.Lock.Runtimes[0].Description.Capabilities["can_inspect_linux_snapshot_density"] = false
		},
		"samples":              func(b *Bundle) { b.Trials[0].Samples = []protocol.Sample{{ElapsedNS: 0}} },
		"root peak RSS":        func(b *Bundle) { b.Trials[0].Observations = []protocol.Observation{{Metric: "process.peak_rss"}} },
		"missing group":        func(b *Bundle) { b.Trials[0].SnapshotDensity.Groups = nil },
		"foreign sample index": func(b *Bundle) { b.Trials[0].SnapshotDensity.Groups[0].Records[1].Boundary.SampleIndex = 1 },
		"wrong count":          func(b *Bundle) { b.Trials[0].SnapshotDensity.Groups[0].Instances = 1 },
		"wrong backend":        func(b *Bundle) { b.Trials[0].SnapshotDensity.Groups[0].Backend = "winch" },
	} {
		t.Run(name, func(t *testing.T) {
			root, b := densityBundleFixture(t)
			mutate(&b)
			if ValidateSnapshotDensityBundle(root, b) == nil {
				t.Fatal("forged sealed density admitted")
			}
		})
	}
}

func TestSnapshotDensityRejectsRepeatedGroupIncarnations(t *testing.T) {
	_, b := densityBundleFixture(t)
	e := b.Trials[0].SnapshotDensity
	encoded, err := json.Marshal(e.Groups[0])
	if err != nil {
		t.Fatal(err)
	}
	var duplicate SnapshotDensityEvidence
	if err := json.Unmarshal(encoded, &duplicate); err != nil {
		t.Fatal(err)
	}
	for index := range duplicate.Records {
		duplicate.Records[index].Boundary.SampleIndex = 1
		for reading := range duplicate.Records[index].Readings {
			duplicate.Records[index].Readings[reading].StartNS += 1000
			duplicate.Records[index].Readings[reading].EndNS += 1000
		}
	}
	e.Groups = append(e.Groups, duplicate)
	r := protocol.RunRequest{Scenario: protocol.SnapshotDensityScenario, Samples: 2, Operations: 1, PhaseBarriers: true}
	if ValidateSnapshotDensityTrial(b.Manifest.Lock.Workloads[0], r, "cranelift", *e) == nil {
		t.Fatal("same live group repeated as fresh samples")
	}
}

func TestNativeSnapshotDensityTrial(t *testing.T) {
	binary := os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_ADAPTER")
	if binary == "" || runtime.GOOS != "linux" {
		t.Skip("requires native Linux density adapter")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		t.Run(backend, func(t *testing.T) {
			root, b := densityBundleFixture(t)
			if err := os.Mkdir(filepath.Join(root, "logs"), 0755); err != nil {
				t.Fatal(err)
			}
			r := b.Manifest.Lock.Runtimes[0]
			r.Command = []string{binary, "--adapter=" + backend}
			c, err := agent.Start(context.Background(), r.Command, filepath.Join(root, "logs", "describe.log"), 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			response, err := c.Call(protocol.Request{Method: "describe"})
			closeErr := c.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
			r.Description = response.Description
			b.Manifest.Lock.Runtimes[0] = r
			b.Manifest.Host.Arch = runtime.GOARCH
			b.Manifest.Lock.Options.Samples = 2
			b.Trials = nil
			w := b.Manifest.Lock.Workloads[0]
			for _, block := range []int{-1, 0} {
				trial := runTrial(context.Background(), root, b.Manifest.Lock.Options, r, w, protocol.SnapshotDensityScenario, block, "native-density-"+backend+string(rune('a'+block+1)))
				if trial.Status != "ok" {
					t.Fatal(trial.Status, trial.Reason)
				}
				if trial.SnapshotDensity == nil || len(trial.SnapshotDensity.Groups) != 2 || len(trial.Samples) != 0 {
					t.Fatal("missing multi-group diagnostics")
				}
				b.Trials = append(b.Trials, trial)
			}
			if err := ValidateSnapshotDensityBundle(root, b); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRetainedSnapshotDensityProduct(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_PRODUCT_EVIDENCE_DIR")
	if root == "" {
		t.Skip("requires completed native product recipe")
	}
	for _, name := range []string{"original", "reproduced"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name)
			if err := Verify(path); err != nil {
				t.Fatal(err)
			}
			b, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateSnapshotDensityBundle(path, b); err != nil {
				t.Fatal(err)
			}
			if len(b.Manifest.Lock.Workloads) != 5 || len(b.Manifest.Lock.Runtimes) != 2 || len(b.Trials) != 30 {
				t.Fatal("missing locked count/backend/trial coverage")
			}
			coverage := map[string]int{}
			groups, readings := 0, 0
			for _, trial := range b.Trials {
				if trial.Status != "ok" || trial.SnapshotDensity == nil || len(trial.Samples) != 0 {
					t.Fatal("missing diagnostic success", trial.ID, trial.Status)
				}
				if trial.Block < 0 {
					continue
				}
				coverage[trial.Runtime+"/"+trial.Workload]++
				groups += len(trial.SnapshotDensity.Groups)
				for _, group := range trial.SnapshotDensity.Groups {
					for _, record := range group.Records {
						readings += len(record.Readings)
					}
				}
			}
			if len(coverage) != 10 || groups != 40 || readings != 1448 {
				t.Fatal("missing measured coverage", coverage, groups, readings)
			}
			for _, count := range coverage {
				if count != 2 {
					t.Fatal("missing independent launch")
				}
			}
		})
	}
}
