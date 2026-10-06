package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestSiteFeatureExportPreservesMeasurementValues(t *testing.T) {
	d := siteFixture()
	name := "features/simd/probe"
	d.Bundle.Manifest.Lock.Workloads[0].ID = name
	for i := range d.Bundle.Trials {
		d.Bundle.Trials[i].Workload = name
	}
	for i := range d.Summaries {
		d.Summaries[i].Workload = name
	}
	for i := range d.MemoryStages {
		d.MemoryStages[i].Workload = name
	}
	for i := range d.CodeRecords {
		d.CodeRecords[i].Workload = name
	}
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "export")
	if err := writeSiteDataset(d, data, []byte("synthetic feature seal"), out); err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest SiteManifest
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	probes, results := 0, 0
	for _, object := range manifest.Objects {
		if object.Kind != "record" {
			continue
		}
		bytes, err := os.ReadFile(filepath.Join(out, "objects", object.SHA256))
		if err != nil {
			t.Fatal(err)
		}
		var record SiteRecord
		if err = json.Unmarshal(bytes, &record); err != nil {
			t.Fatal(err)
		}
		switch record.Kind {
		case "feature-probe":
			var probe SiteFeatureProbe
			if err = json.Unmarshal(record.Data, &probe); err != nil {
				t.Fatal(err)
			}
			if probe.TrialCount != 1 || len(probe.Scenarios) != 1 || probe.Scenarios[0].Outcomes["ok"] != 1 || len(probe.Evidence) != 1 {
				t.Fatal("feature source counts drift", probe)
			}
			probes++
		case "result":
			results++
		}
	}
	if probes != 1 || results != 3 {
		t.Fatal("feature projection replaced measurements", probes, results)
	}
}

func TestSiteFeatureRecordedTrialsAndBounds(t *testing.T) {
	d := siteFixture()
	d.Bundle.Manifest.Lock.Workloads = append(d.Bundle.Manifest.Lock.Workloads, protocol.Workload{ID: "features/simd/probe"}, protocol.Workload{ID: "features/simd/baseline", Provenance: json.RawMessage(`{"baseline":true}`)}, protocol.Workload{ID: "features/simd/unvisited"})
	refs := map[string]string{}
	d.Bundle.Trials = nil
	for i := 0; i < 1500; i++ {
		status := "ok"
		if i%3 == 0 {
			status = "unsupported"
		}
		if i == 1 {
			status = "crashed"
		}
		trial := experiment.Trial{ID: fmt.Sprint(i), Runtime: "engine", Workload: "features/simd/probe", Profile: "timing", Scenario: "steady", Status: status, Block: 0}
		d.Bundle.Trials = append(d.Bundle.Trials, trial)
		refs[d.Bundle.Manifest.ID+"\x00"+trial.ID] = siteHash([]byte("timing:" + trial.ID))
	}
	extra := []experiment.Bundle{{Manifest: experiment.Manifest{ID: "diagnostic-pass"}, Trials: []experiment.Trial{{ID: "0", Runtime: "engine", Workload: "features/simd/probe", Profile: "memory", Scenario: "steady", Status: "ok", Block: 0}}}}
	extra[0].Manifest = d.Bundle.Manifest
	extra[0].Manifest.ID = "diagnostic-pass"
	extra[0].Manifest.Lock.Options.Profile = "memory"
	refs["diagnostic-pass\x000"] = siteHash([]byte("memory:0"))
	objects := map[string][]byte{}
	object := func(kind string, v any) (string, error) {
		bytes, e := siteJSON(v)
		hash := siteHash(bytes)
		objects[hash] = bytes
		return hash, e
	}
	probes := map[string]SiteFeatureProbe{}
	record := func(kind, id string, v any) error {
		if kind != "feature-probe" {
			t.Fatal(kind)
		}
		bytes, e := siteJSON(v)
		if e != nil {
			return e
		}
		if id != siteHash(bytes) {
			t.Fatal("probe identity")
		}
		probes[v.(SiteFeatureProbe).Workload] = v.(SiteFeatureProbe)
		return nil
	}
	h := siteHash([]byte("fixture"))
	ids := map[string]string{"engine": h}
	contracts := map[string]string{"features/simd/probe": h, "features/simd/unvisited": h}
	if e := siteFeatureProbes(d, extra, h, h, ids, ids, contracts, refs, object, record); e != nil {
		t.Fatal(e)
	}
	if len(probes) != 2 {
		t.Fatal("baseline or ordinary workload leaked", len(probes))
	}
	p := probes["features/simd/probe"]
	if p.TrialCount != 1501 || len(p.Scenarios) != 2 || len(p.Evidence) != 1 {
		t.Fatal("pass population or bounded roots", p)
	}
	for _, group := range p.Scenarios {
		if group.Profile == "timing" && (group.TrialCount != 1500 || group.Outcomes["unsupported"] != 500 || group.Outcomes["crashed"] != 1 || group.Outcomes["ok"] != 999) {
			t.Fatal("outcome drift", group)
		}
	}
	if probes["features/simd/unvisited"].TrialCount != 0 || len(probes["features/simd/unvisited"].Evidence) != 0 {
		t.Fatal("invented unvisited evidence")
	}
	seen := []string{}
	var walk func(string)
	walk = func(id string) {
		var index struct {
			References []string `json:"references"`
		}
		if bytes, ok := objects[id]; ok {
			if e := json.Unmarshal(bytes, &index); e != nil {
				t.Fatal(e)
			}
			for _, ref := range index.References {
				walk(ref)
			}
		} else {
			seen = append(seen, id)
		}
	}
	walk(p.Evidence[0])
	if len(seen) != 1501 || seen[0] != refs[d.Bundle.Manifest.ID+"\x000"] || seen[1500] != refs["diagnostic-pass\x000"] {
		t.Fatal("lost pass-qualified trial references")
	}
	// A pass can contain some matching cells without matching this probe's
	// exact workload contract. The producer's existing join policy decides.
	extra[0].Manifest.Lock.Workloads = append([]protocol.Workload{}, extra[0].Manifest.Lock.Workloads...)
	extra[0].Manifest.Lock.Workloads[1].SHA256 = "changed-contract"
	if e := siteFeatureProbes(d, extra, h, h, ids, ids, contracts, refs, object, record); e != nil {
		t.Fatal(e)
	}
	if probe := probes["features/simd/probe"]; probe.TrialCount != 1500 || len(probe.Scenarios) != 1 {
		t.Fatal("unmatched contract joined", probe)
	}
}
