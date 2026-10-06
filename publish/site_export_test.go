package publish

import (
	"bytes"
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
	bundle.Manifest.Lock.Options.Profile = "memory"
	codeBundle.Manifest.Lock.Options.Profile = "code"
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
				var metadata map[string]json.RawMessage
				_ = json.Unmarshal(record.Data, &metadata)
				if metadata["exporterIdentity"] != nil {
					t.Fatal("exporter binary contaminated scientific report identity")
				}

				var report struct {
					PassContexts []string `json:"passContexts"`
				}
				if err = json.Unmarshal(record.Data, &report); err != nil {
					t.Fatal(err)
				}
				executable, _ := os.Executable()
				digest, err := experiment.DigestFile(executable)
				if err != nil || manifest.ExporterIdentity == nil || manifest.ExporterIdentity.BinarySHA256 != digest || len(report.PassContexts) != 1 {
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

func TestSiteExportLargeEvidencePreservesExactJSON(t *testing.T) {
	d := siteFixture()
	// A pass context, trial log and one phase row each exceed the normal chunk.
	d.Bundle.Manifest.Lock.Options.Suite = strings.Repeat("λ 🦀 \\", 70000)
	d.Bundle.Trials[0].Log = strings.Repeat("large diagnostic λ ", 30000)
	d.Bundle.Trials[0].PhaseEvents = []experiment.PhaseRecord{{Observations: []protocol.Observation{{Reason: strings.Repeat("phase diagnostic ", 20000)}}}}
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if err := writeSiteDataset(d, data, []byte("seal"), out); err != nil {
		t.Fatal(err)
	}
	var manifest SiteManifest
	if err := experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	resources := 0
	for _, object := range manifest.Objects {
		b, err := os.ReadFile(filepath.Join(out, "objects", object.SHA256))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > SiteChunkBytes {
			t.Fatal("oversized emitted object")
		}
		var resource struct {
			Kind       string
			Schema     int
			Bytes      int
			SHA256     string
			References []string
		}
		if json.Unmarshal(b, &resource) != nil || resource.Kind != "json-resource" {
			continue
		}
		resources++
		assembled := []byte{}
		for _, ref := range resource.References {
			var part struct {
				Kind   string
				Schema int
				Text   string
			}
			if err = experiment.ReadJSON(filepath.Join(out, "objects", ref), &part); err != nil {
				t.Fatal(err)
			}
			if part.Kind != "json-fragment" || part.Schema != 1 || len(part.Text) > SiteFragmentBytes {
				t.Fatal("invalid fragment")
			}
			assembled = append(assembled, part.Text...)
		}
		if len(assembled) != resource.Bytes || siteHash(assembled) != resource.SHA256 || !json.Valid(assembled) {
			t.Fatal("changed reassembled JSON")
		}
		if len(assembled) > 0 && assembled[0] == '[' {
			want, _ := json.Marshal(d.Bundle.Trials[0].PhaseEvents)
			if string(assembled) != string(want) {
				t.Fatal("changed oversized phase row")
			}
			continue
		}
		var original map[string]json.RawMessage
		_ = json.Unmarshal(assembled, &original)
		if string(original["kind"]) == `"pass-context"` {
			want, _ := json.Marshal(d.Bundle.Manifest)
			if string(original["manifest"]) != string(want) {
				t.Fatal("changed large pass recipe")
			}
		} else if string(original["kind"]) == `"trial-details"` {
			var details struct{ Log string }
			_ = json.Unmarshal(original["data"], &details)
			if details.Log != d.Bundle.Trials[0].Log {
				t.Fatal("changed large trial diagnostic")
			}
		}
	}
	if resources < 3 {
		t.Fatal("large evidence was not fragmented")
	}
}

func nativeSiteFixture() (Dataset, experiment.Bundle, []byte) {
	d := siteFixture()
	module := siteHash([]byte("synthetic input module"))
	d.Bundle.Manifest.Lock.Workloads[0].SHA256 = module
	native := bytes.Repeat([]byte{0x90, 0xc3}, 200000)
	image := &protocol.CodeImage{Version: 2, ModuleSHA256: module, SHA256: siteHash(native), Architecture: "amd64", Backend: "cranelift", Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "engine_reported", Data: native, Functions: []protocol.CodeFunction{{ModuleIndex: 0, WasmIndex: 7, Offset: 0, Length: 32, Tier: "cranelift"}}}
	count := len(native)
	d.CodeRecords[0].Status = "available"
	d.CodeRecords[0].ImageBytes = &count
	size := uint64(count)
	d.CodeRecords[0].SizeBytes = &size
	bundle := experiment.Bundle{Manifest: experiment.Manifest{ID: "code-pass"}, Trials: []experiment.Trial{{ID: "code-0", Runtime: "engine", Workload: "fixture/a", Profile: "code", Scenario: "compile", Status: "ok", CodeImage: image}}}
	bundle.Manifest.Lock.Options.Profile = "code"
	return d, bundle, native
}

func TestSiteExportNativeBinaryAndFunctionResources(t *testing.T) {
	d, bundle, native := nativeSiteFixture()
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if err := writeSiteDataset(d, data, []byte("synthetic seal"), out, bundle); err != nil {
		t.Fatal(err)
	}
	var manifest SiteManifest
	if err := experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	foundBinary, foundDescriptor := false, false
	for _, object := range manifest.Objects {
		b, err := os.ReadFile(filepath.Join(out, "objects", object.SHA256))
		if err != nil {
			t.Fatal(err)
		}
		if object.Kind == "binary" {
			foundBinary = true
			if !bytes.Equal(b, native) || object.SHA256 != bundle.Trials[0].CodeImage.SHA256 || object.Bytes != len(native) {
				t.Fatal("native original changed")
			}
			continue
		}
		if len(b) > SiteChunkBytes {
			t.Fatal("unbounded JSON metadata")
		}
		if object.Kind != "record" {
			continue
		}
		var record SiteRecord
		_ = json.Unmarshal(b, &record)
		if record.Kind != "artifact" {
			continue
		}
		foundDescriptor = true
		if len(record.Data)+512 > 10*1024 {
			t.Fatal("oversized artifact descriptor")
		}
		var descriptor struct {
			MeasurementAvailable bool `json:"measurementAvailable"`
			Content              struct {
				Status, SHA256 string
				Bytes          int
			} `json:"content"`
			Inspection struct {
				Status, Metadata string
				Disassembly      struct{ Status string }
			} `json:"inspection"`
		}
		if err = json.Unmarshal(record.Data, &descriptor); err != nil {
			t.Fatal(err)
		}
		if !descriptor.MeasurementAvailable || descriptor.Content.Status != "available" || descriptor.Content.SHA256 != siteHash(native) || descriptor.Content.Bytes != len(native) || descriptor.Inspection.Status != "available" || descriptor.Inspection.Disassembly.Status != "unavailable" {
			t.Fatal("availability or provenance changed")
		}
		var metadata struct {
			Image                protocol.CodeImage
			Functions            []string
			FunctionIndexVersion string
			FunctionShards       []struct {
				SHA256 string
				Count  int
			}
		}
		if err = experiment.ReadJSON(filepath.Join(out, "objects", descriptor.Inspection.Metadata), &metadata); err != nil {
			t.Fatal(err)
		}
		if len(metadata.Image.Data) != 0 || len(metadata.Image.Functions) != 0 || len(metadata.Functions) != 1 {
			t.Fatal("inline image or function preload")
		}
		var functions []protocol.CodeFunction
		if err = experiment.ReadJSON(filepath.Join(out, "objects", metadata.Functions[0]), &functions); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(functions, bundle.Trials[0].CodeImage.Functions) {
			t.Fatal("function attribution changed")
		}
		if metadata.FunctionIndexVersion != "producer-order-v1" || len(metadata.FunctionShards) != 1 || metadata.FunctionShards[0].SHA256 != metadata.Functions[0] || metadata.FunctionShards[0].Count != len(functions) {
			t.Fatal("function counts lost attribution")
		}
	}
	if !foundBinary || !foundDescriptor {
		t.Fatal("missing admitted native content")
	}
	if fixture := os.Getenv("WASMFYI_NATIVE_FIXTURE_OUT"); fixture != "" {
		if err := os.CopyFS(fixture, os.DirFS(out)); err != nil {
			t.Fatal(err)
		}
	}

}

func TestSiteExportRejectsNativeIdentityMismatch(t *testing.T) {
	for _, kind := range []string{"hash", "module", "runtime", "profile", "size"} {
		t.Run(kind, func(t *testing.T) {
			d, bundle, _ := nativeSiteFixture()
			switch kind {
			case "hash":
				bundle.Trials[0].CodeImage.SHA256 = strings.Repeat("a", 64)
			case "module":
				bundle.Trials[0].CodeImage.ModuleSHA256 = strings.Repeat("b", 64)
			case "runtime":
				bundle.Trials[0].Runtime = "different"
			case "profile":
				bundle.Trials[0].Profile = "timing"
			case "size":
				wrong := 1
				d.CodeRecords[0].ImageBytes = &wrong
			}
			data, _ := json.Marshal(d)
			out := filepath.Join(t.TempDir(), "site")
			if err := writeSiteDataset(d, data, []byte("seal"), out, bundle); err == nil {
				t.Fatal("accepted native identity mismatch")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("failed export left files")
			}
		})
	}
}

func TestSiteExportDoesNotExposeWithheldNativeImage(t *testing.T) {
	d, bundle, _ := nativeSiteFixture()
	d.CodeRecords[0].Status = "withheld_host_mismatch"
	d.CodeRecords[0].ImageBytes = nil
	d.CodeRecords[0].SizeBytes = nil
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if err := writeSiteDataset(d, data, []byte("seal"), out, bundle); err != nil {
		t.Fatal(err)
	}
	var manifest SiteManifest
	if err := experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	for _, object := range manifest.Objects {
		if object.Kind == "binary" {
			t.Fatal("published withheld bytes")
		}
	}
}

func TestSiteExportEmptyNativeOriginal(t *testing.T) {
	d, bundle, _ := nativeSiteFixture()
	image := bundle.Trials[0].CodeImage
	image.Version = 1
	image.FunctionAttribution = "unavailable"
	image.Functions = nil
	image.Data = []byte{}
	image.SHA256 = siteHash(nil)
	zero := 0
	size := uint64(0)
	d.CodeRecords[0].ImageBytes = &zero
	d.CodeRecords[0].SizeBytes = &size
	data, _ := json.Marshal(d)
	out := filepath.Join(t.TempDir(), "site")
	if err := writeSiteDataset(d, data, []byte("seal"), out, bundle); err != nil {
		t.Fatal(err)
	}
	var manifest SiteManifest
	if err := experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, object := range manifest.Objects {
		if object.Kind == "binary" && object.SHA256 == siteHash(nil) && object.Bytes == 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("empty original became unavailable content")
	}
}

func TestSiteExportPreservesDerivedSourceSections(t *testing.T) {
	d := siteFixture()
	raw, _ := json.Marshal(d)
	var source map[string]json.RawMessage
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	// Future derived fields and unsafe JSON integers must survive as exact source
	// values, independently of the typed summary projection.
	source["future_derived"] = json.RawMessage(`{"counter":9007199254740993,"status":"unavailable","reason":"source reason","launches":[1,2]}`)
	large, _ := json.Marshal(map[string]any{"diagnostic": strings.Repeat("λ diagnostic ", 30000), "coverage": "partial"})
	source["future_large"] = large
	source["future_analysis_version"] = json.RawMessage(`"method-7"`)
	data, _ := json.Marshal(source)
	out := filepath.Join(t.TempDir(), "site")
	if err := writeSiteDataset(d, data, []byte("seal"), out); err != nil {
		t.Fatal(err)
	}
	var manifest SiteManifest
	if err := experiment.ReadJSON(filepath.Join(out, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	var sections map[string]string
	var versions map[string]json.RawMessage
	for _, o := range manifest.Objects {
		if o.Bytes > SiteChunkBytes {
			t.Fatal("unbounded analysis object")
		}
		if o.Kind != "record" {
			continue
		}
		var r SiteRecord
		if err := experiment.ReadJSON(filepath.Join(out, "objects", o.SHA256), &r); err != nil {
			t.Fatal(err)
		}
		if r.Kind == "report" {
			var report struct {
				SourceSchema           int
				AnalysisSectionVersion string
				AnalysisSections       map[string]string
				Versions               map[string]json.RawMessage
			}
			if err := json.Unmarshal(r.Data, &report); err != nil {
				t.Fatal(err)
			}
			if report.AnalysisSectionVersion != "source-fields-v1" || report.SourceSchema != d.Schema {
				t.Fatal("missing section format")
			}
			sections, versions = report.AnalysisSections, report.Versions
		}
	}
	read := func(id string) []byte {
		b, err := os.ReadFile(filepath.Join(out, "objects", id))
		if err != nil {
			t.Fatal(err)
		}
		if siteHash(b) != id {
			t.Fatal("digest mismatch")
		}
		return b
	}
	for _, field := range []string{"future_derived", "future_large", "throughput", "scaling", "memory_timelines", "counter_display", "cpu_stacks"} {
		id := sections[field]
		if id == "" {
			t.Fatal("source section lost", field)
		}
		b := read(id)
		var resource struct {
			Kind       string
			Bytes      int
			SHA256     string
			References []string
		}
		if err := json.Unmarshal(b, &resource); err != nil {
			t.Fatal(err)
		}
		if resource.Kind == "json-resource" {
			assembled := []byte{}
			for _, ref := range resource.References {
				var fragment struct{ Text string }
				if err := json.Unmarshal(read(ref), &fragment); err != nil {
					t.Fatal(err)
				}
				assembled = append(assembled, fragment.Text...)
			}
			if len(assembled) != resource.Bytes || siteHash(assembled) != resource.SHA256 {
				t.Fatal("section resource integrity")
			}
			b = assembled
		}
		var section struct {
			Kind, ReportID, Field string
			Schema                int
			Data                  json.RawMessage
		}
		if err := json.Unmarshal(b, &section); err != nil {
			t.Fatal(err)
		}
		if section.Kind != "report-analysis" || section.Schema != 1 || section.ReportID != manifest.ReportID || section.Field != field || !bytes.Equal(section.Data, source[field]) {
			t.Fatal("source section changed", field)
		}
	}
	if sections["bundle"] != "" || sections["summaries"] != "" || sections["future_analysis_version"] != "" || string(versions["future_analysis_version"]) != `"method-7"` {
		t.Fatal("typed fields or versions duplicated into analysis sections")
	}
}
