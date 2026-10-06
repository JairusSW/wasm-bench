package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/metrics"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestSiteExportVerifiedReport(t *testing.T) {
	source := replayFixtureReport(t, true)
	before, e := experiment.DigestFile(filepath.Join(source, "checksums.json"))
	if e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "site")
	if e = ExportSite(source, out); e != nil {
		t.Fatal(e)
	}
	var dataset Dataset
	if e = experiment.ReadJSON(filepath.Join(source, "data.json"), &dataset); e != nil {
		t.Fatal(e)
	}
	var manifest SiteManifest
	if e = experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); e != nil {
		t.Fatal(e)
	}
	matched := 0
	for _, object := range manifest.Objects {
		if object.Kind != "record" {
			continue
		}
		var record SiteRecord
		if e = experiment.ReadJSON(filepath.Join(out, "objects", object.SHA256), &record); e != nil {
			t.Fatal(e)
		}
		if record.Kind != "result" {
			continue
		}
		var result struct {
			Runtime, Workload, Scenario, Profile, Metric string
			Summary                                      map[string]json.RawMessage
		}
		if e = json.Unmarshal(record.Data, &result); e != nil {
			t.Fatal(e)
		}
		if result.Metric != "time.wall" {
			continue
		}
		for _, summary := range dataset.Summaries {
			if summary.Runtime != result.Runtime || summary.Workload != result.Workload || summary.Scenario != result.Scenario || summary.Profile != result.Profile {
				continue
			}
			b, _ := json.Marshal(summary)
			var want map[string]json.RawMessage
			_ = json.Unmarshal(b, &want)
			delete(want, "launch_medians")
			delete(want, "warmup_diagnostics")
			if !reflect.DeepEqual(want, result.Summary) {
				t.Fatal("verified source summary changed during export")
			}
			matched++
		}
	}
	if matched != len(dataset.Summaries) {
		t.Fatalf("exported %d of %d summaries", matched, len(dataset.Summaries))
	}
	after, _ := experiment.DigestFile(filepath.Join(source, "checksums.json"))
	if after != before {
		t.Fatal("changed sealed report")
	}
	if e = ExportSite(source, filepath.Join(source, "nested")); e == nil {
		t.Fatal("accepted nested export")
	}
	if e = os.WriteFile(filepath.Join(source, "data.json"), []byte("{}"), 0644); e != nil {
		t.Fatal(e)
	}
	if e = ExportSite(source, filepath.Join(t.TempDir(), "corrupt")); e == nil {
		t.Fatal("accepted corrupt report")
	}
}
func siteFixture() Dataset {
	zero := 0.0
	size := uint64(1234)
	created := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	d := Dataset{Schema: 1, AnalysisVersion: analysis.Version, Metrics: metrics.Registry, Bundle: experiment.Bundle{Manifest: experiment.Manifest{ID: "synthetic-single-launch", Created: created, Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "timing"}, Runtimes: []experiment.Runtime{{ID: "engine"}}, Workloads: []protocol.Workload{{ID: "fixture/a"}}}}, Trials: []experiment.Trial{{ID: "launch-0", Runtime: "engine", Workload: "fixture/a", Scenario: "steady", Profile: "timing", Block: 0, Status: "ok", Samples: []protocol.Sample{{}, {}, {}}}}}, Summaries: []analysis.Summary{{Runtime: "engine", Workload: "fixture/a", Scenario: "steady", Profile: "timing", Launches: 1, RecordedSamples: 3, Median: &zero, LaunchMedians: map[int]float64{0: 0}, Uncertainty: "unavailable"}}, MemorySource: &MemorySource{ID: "synthetic-single-launch", Profile: "timing"}, MemoryStages: []MemoryStage{{Runtime: "engine", Workload: "fixture/a", Scenario: "steady", Metric: "process.peak_rss", SourceRun: "synthetic-single-launch", SourceProfile: "timing", Median: &zero, Launches: 1, Trials: []string{"launch-0"}, LaunchValues: []MemoryLaunch{{TrialID: "launch-0", Bytes: 0}}}}, CodeSource: &CodeSource{ID: "code-pass", Profile: "code"}, CodeRecords: []CodeRecord{{Runtime: "engine", Workload: "fixture/a", Trial: "code-0", Status: "unavailable", SizeBytes: &size, SizeNote: "engine-reported"}}}
	d.Metrics = nil
	for _, def := range metrics.Registry {
		if def.Name == "time.wall" || def.Name == "process.peak_rss" || def.Name == "native.code_size" {
			d.Metrics = append(d.Metrics, def)
		}
	}
	return d
}
func TestSiteExportParityAndBounds(t *testing.T) {
	d := siteFixture()
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if e := writeSiteDataset(d, data, []byte("synthetic fixture seal"), out); e != nil {
		t.Fatal(e)
	}
	manifestBytes, e := os.ReadFile(filepath.Join(out, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m SiteManifest
	if e = json.Unmarshal(manifestBytes, &m); e != nil {
		t.Fatal(e)
	}
	results := 0
	artifacts := 0
	for _, o := range m.Objects {
		b, e := os.ReadFile(filepath.Join(out, "objects", o.SHA256))
		if e != nil {
			t.Fatal(e)
		}
		if len(b) != o.Bytes || len(b) > SiteChunkBytes || siteHash(b) != o.SHA256 {
			t.Fatal("invalid chunk")
		}
		if o.Kind != "record" {
			continue
		}
		var r SiteRecord
		_ = json.Unmarshal(b, &r)
		if r.Kind == "result" {
			results++
			var v struct {
				Metric   string                     `json:"metric"`
				Summary  map[string]json.RawMessage `json:"summary"`
				Evidence []string                   `json:"evidence"`
			}
			_ = json.Unmarshal(r.Data, &v)
			if v.Metric == "time.wall" {
				if string(v.Summary["median_ns_per_operation"]) != "0" || string(v.Summary["ci95_low"]) != "null" || v.Summary["launch_medians"] != nil {
					t.Fatal("value/interval/summary drift")
				}
			}
			if v.Metric == "process.peak_rss" && len(v.Evidence) != 2 {
				t.Fatal("memory lost launch evidence")
			}
		}
		if r.Kind == "artifact" {
			artifacts++
			if !strings.Contains(string(r.Data), `"measurementAvailable":true`) || strings.Contains(string(r.Data), `"sha256"`) {
				t.Fatal("size-only artifact fabricated bytes")
			}
		}
	}
	if results != 3 || artifacts != 1 {
		t.Fatalf("results=%d artifacts=%d", results, artifacts)
	}
	second := filepath.Join(t.TempDir(), "site")
	if e = writeSiteDataset(d, data, []byte("synthetic fixture seal"), second); e != nil {
		t.Fatal(e)
	}
	other, _ := os.ReadFile(filepath.Join(second, "manifest.json"))
	if !reflect.DeepEqual(manifestBytes, other) {
		t.Fatal("nondeterministic export")
	}
	if fixture := os.Getenv("WASMFYI_FIXTURE_OUT"); fixture != "" {
		if e = os.CopyFS(fixture, os.DirFS(out)); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSiteExportCleansFailedOutput(t *testing.T) {
	d := siteFixture()
	d.Bundle.Manifest.Lock.Runtimes[0].UnavailableReason = strings.Repeat("x", SiteChunkBytes)
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if e := writeSiteDataset(d, data, []byte("seal"), out); e == nil {
		t.Fatal("oversized record accepted")
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("failed export left output")
	}
}

func TestSiteExportSeparateMemoryAndExactSize(t *testing.T) {
	d := siteFixture()
	d.MemorySource = &MemorySource{ID: "memory-pass", Profile: "memory"}
	d.MemoryStages[0].SourceProfile = ""
	d.MemoryStages[0].SourceRun = ""
	d.MemoryStages[0].Trials = []string{"memory-0"}
	d.MemoryStages[0].LaunchValues = []MemoryLaunch{{TrialID: "memory-0", Bytes: 0}}
	size := uint64(1<<53 + 1)
	d.CodeRecords[0].SizeBytes = &size
	bundle := experiment.Bundle{Manifest: experiment.Manifest{ID: "memory-pass"}, Trials: []experiment.Trial{{ID: "memory-0", Runtime: "engine", Workload: "fixture/a", Profile: "memory", Scenario: "steady"}}}
	codeBundle := experiment.Bundle{Manifest: experiment.Manifest{ID: "code-pass"}, Trials: []experiment.Trial{{ID: "code-0", Runtime: "engine", Workload: "fixture/a", Profile: "code", Scenario: "compile"}}}
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if e := writeSiteDataset(d, data, []byte("synthetic seal"), out, bundle, codeBundle); e != nil {
		t.Fatal(e)
	}
	var manifest SiteManifest
	if e := experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); e != nil {
		t.Fatal(e)
	}
	memoryFound, sizeFound := false, false
	for _, object := range manifest.Objects {
		if object.Kind != "record" {
			continue
		}
		var record SiteRecord
		if e := experiment.ReadJSON(filepath.Join(out, "objects", object.SHA256), &record); e != nil {
			t.Fatal(e)
		}
		if record.Kind == "result" {
			var result struct {
				Metric, Profile string
				Summary         map[string]json.RawMessage
				Evidence        []string
			}
			_ = json.Unmarshal(record.Data, &result)
			if result.Metric == "process.peak_rss" {
				memoryFound = true
				if result.Profile != "memory" || len(result.Evidence) != 2 {
					t.Fatal("separate memory pass provenance lost")
				}
			}
			if result.Metric == "native.code_size" {
				sizeFound = true
				if len(result.Evidence) != 1 {
					t.Fatal("code pass trial lost")
				}
				var trial struct {
					PassID  string `json:"passId"`
					TrialID string `json:"trialId"`
				}
				if err := experiment.ReadJSON(filepath.Join(out, "objects", result.Evidence[0]), &trial); err != nil || trial.PassID != "code-pass" || trial.TrialID != "code-0" {
					t.Fatal("code pass source changed", err)
				}
				if string(result.Summary["size_bytes"]) != `"9007199254740993"` {
					t.Fatal("unsafe integer rounded")
				}
			}
		}
	}
	if !memoryFound || !sizeFound {
		t.Fatal("missing memory/size result")
	}
}

func TestSiteExportPassAndTrialEvidence(t *testing.T) {
	d := siteFixture()
	d.Bundle.Manifest.Lock.Options.Launches = 1
	d.Bundle.Manifest.Lock.Options.Samples = 3
	d.Bundle.Trials[0].AdapterSamples = []protocol.Sample{{}}
	d.Bundle.Trials[0].PhaseEvents = []experiment.PhaseRecord{{}}
	d.Bundle.Trials[0].Log = "producer diagnostic"
	d.Bundle.Trials[0].DurationNS = 42
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if err := writeSiteDataset(d, data, []byte("seal"), out); err != nil {
		t.Fatal(err)
	}
	var manifest SiteManifest
	if err := experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, obj := range manifest.Objects {
		b, err := os.ReadFile(filepath.Join(out, "objects", obj.SHA256))
		if err != nil {
			t.Fatal(err)
		}
		if obj.Kind == "record" {
			var record SiteRecord
			if err = json.Unmarshal(b, &record); err != nil {
				t.Fatal(err)
			}
			if record.Kind == "report" {
				var report struct {
					ExporterIdentity struct {
						BinarySHA256 string `json:"binarySha256"`
					} `json:"exporterIdentity"`
					PassContexts []string `json:"passContexts"`
				}
				if err = json.Unmarshal(record.Data, &report); err != nil {
					t.Fatal(err)
				}
				executable, _ := os.Executable()
				digest, err := experiment.DigestFile(executable)
				if err != nil || report.ExporterIdentity.BinarySHA256 != digest || len(report.PassContexts) != 1 {
					t.Fatal("exporter/pass provenance lost")
				}
			}
			continue
		}
		var evidence map[string]json.RawMessage
		if json.Unmarshal(b, &evidence) != nil {
			continue
		}
		var kind string
		_ = json.Unmarshal(evidence["kind"], &kind)
		kinds[kind] = true
		switch kind {
		case "pass-context":
			want, _ := json.Marshal(d.Bundle.Manifest)
			if string(evidence["manifest"]) != string(want) {
				t.Fatal("pass recipe changed")
			}
		case "trial-details":
			var details map[string]json.RawMessage
			_ = json.Unmarshal(evidence["data"], &details)
			if string(details["duration_ns"]) != "42" || string(details["log"]) != `"producer diagnostic"` || details["samples"] != nil {
				t.Fatal("trial detail lost or eager samples")
			}
		case "summary-diagnostics":
			var details map[string]json.RawMessage
			_ = json.Unmarshal(evidence["data"], &details)
			want, _ := json.Marshal(d.Summaries[0].LaunchMedians)
			if string(details["launch_medians"]) != string(want) {
				t.Fatal("launch diagnostics changed")
			}
		default:
			if evidence["trialId"] != nil {
				var refs []string
				_ = json.Unmarshal(evidence["references"], &refs)
				if len(refs) != 4 || string(evidence["adapterSamples"]) == "[]" || string(evidence["phaseEvents"]) == "[]" {
					t.Fatal("trial resource links lost")
				}
			}
		}
	}
	for _, kind := range []string{"pass-context", "trial-details", "summary-diagnostics"} {
		if !kinds[kind] {
			t.Fatalf("missing %s", kind)
		}
	}
}

func TestSiteExportPagedInventoryCoverage(t *testing.T) {
	d := siteFixture()
	original := d.Bundle.Trials[0]
	d.Bundle.Trials = nil
	for i := 0; i < 2000; i++ {
		trial := original
		trial.ID = fmt.Sprintf("launch-%d", i)
		trial.Block = i
		d.Bundle.Trials = append(d.Bundle.Trials, trial)
	}
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if err := writeSiteDataset(d, data, []byte("large synthetic seal"), out); err != nil {
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
	if len(manifest.Objects) != 0 || len(manifest.InventoryPages) < 2 || len(manifestBytes) > 50*1024 {
		t.Fatal("large inventory was embedded or unbounded")
	}
	seen := map[string]bool{}
	trials := map[string]bool{}
	resultCount := 0
	for _, descriptor := range manifest.InventoryPages {
		b, err := os.ReadFile(filepath.Join(out, "objects", descriptor.SHA256))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) != descriptor.Bytes || siteHash(b) != descriptor.SHA256 || len(b) > SiteChunkBytes {
			t.Fatal("invalid inventory representation")
		}
		var page siteInventoryPage
		if err = json.Unmarshal(b, &page); err != nil {
			t.Fatal(err)
		}
		if page.Schema != 1 || len(page.Objects) != descriptor.Objects || len(page.Objects) > SiteInventoryObjects {
			t.Fatal("invalid page count")
		}
		var contentBytes int64
		for _, object := range page.Objects {
			if seen[object.SHA256] {
				t.Fatal("duplicate payload across pages")
			}
			seen[object.SHA256] = true
			contentBytes += int64(object.Bytes)
			payload, err := os.ReadFile(filepath.Join(out, "objects", object.SHA256))
			if err != nil {
				t.Fatal(err)
			}
			if len(payload) != object.Bytes || len(payload) > SiteChunkBytes || siteHash(payload) != object.SHA256 {
				t.Fatal("invalid payload")
			}
			if object.Kind == "evidence" {
				var envelope struct {
					TrialID string `json:"trialId"`
				}
				if json.Unmarshal(payload, &envelope) == nil && envelope.TrialID != "" {
					trials[envelope.TrialID] = true
				}
			} else {
				var record SiteRecord
				if err = json.Unmarshal(payload, &record); err != nil {
					t.Fatal(err)
				}
				if record.Kind == "result" {
					resultCount++
				}
			}
		}
		if contentBytes != descriptor.ContentBytes {
			t.Fatal("declared payload reservation differs")
		}
	}
	if len(trials) != len(d.Bundle.Trials) || resultCount != 3 {
		t.Fatal("lost trials or numerical summaries during inventory paging")
	}
	for _, trial := range d.Bundle.Trials {
		if !trials[trial.ID] {
			t.Fatalf("lost trial %s", trial.ID)
		}
	}
}
