package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func nativeComparisonFixture() (nativeComparisonInput, nativeComparisonInput) {
	digest := strings.Repeat("a", 64)
	w := protocol.Workload{ID: "w", SHA256: digest, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: []uint64{1}}}
	makeSide := func(id, backend string, data []byte, functions []protocol.CodeFunction) nativeComparisonInput {
		image := &protocol.CodeImage{Version: 2, ModuleSHA256: digest, SHA256: corpus.Hash(data), Data: data, Architecture: "amd64", Backend: backend, Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "engine_reported", Functions: functions}
		t := experiment.Trial{ID: id, Runtime: id, Workload: "w", Profile: "code", Scenario: "compile", Status: "ok", CodeImage: image}
		m := experiment.Manifest{ID: id, Kind: "measurement", Lock: experiment.Lock{Protocol: 1, Options: experiment.Options{Profile: "code"}, Runtimes: []experiment.Runtime{{ID: id}}, Workloads: []protocol.Workload{w}}}
		return nativeComparisonInput{bundle: experiment.Bundle{Manifest: m, Trials: []experiment.Trial{t}}, report: NativeExport{Records: []NativeExportRecord{nativeRecord(t, 0)}}}
	}
	a := makeSide("a", "cranelift", []byte{1, 2, 3, 4}, []protocol.CodeFunction{{WasmIndex: 7, Length: 2, Tier: "cranelift"}, {WasmIndex: 8, Offset: 2, Length: 2, Tier: "cranelift"}})
	b := makeSide("b", "winch", []byte{3, 4, 1, 9, 5}, []protocol.CodeFunction{{WasmIndex: 8, Length: 2, Tier: "winch"}, {WasmIndex: 7, Offset: 2, Length: 3, Tier: "winch"}})
	return a, b
}

func TestNativeComparisonMatchesFullIndicesAndExactRangeBytes(t *testing.T) {
	a, b := nativeComparisonFixture()
	x, err := compareNativeInputs(a, b, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Results) != 1 || x.Results[0].Status != "matched" || len(x.Results[0].Functions) != 2 {
		t.Fatal(x)
	}
	first, second := x.Results[0].Functions[0], x.Results[0].Functions[1]
	if first.WasmIndex != 7 || first.Status != "changed" || *first.SizeDelta != 1 || *first.FirstDifferenceOffset != 1 || *first.BytesEqual {
		t.Fatal(first)
	}
	if second.WasmIndex != 8 || second.Status != "identical_bytes" || *second.SizeDelta != 0 || !*second.BytesEqual || second.FirstDifferenceOffset != nil {
		t.Fatal(second)
	}
	if second.Baseline.Function.Offset == second.Candidate.Function.Offset {
		t.Fatal("fixture must exercise relocation of identical range bytes")
	}
	if x.BaselineConfiguration.ID != "a" || x.CandidateConfiguration.ID != "b" {
		t.Fatal("lost configurations")
	}
}

func TestNativeComparisonKeepsMissingAndIncomparableEvidence(t *testing.T) {
	for _, mode := range []string{"artifact", "oracle", "function", "legacy", "block"} {
		t.Run(mode, func(t *testing.T) {
			a, b := nativeComparisonFixture()
			switch mode {
			case "artifact":
				b.bundle.Manifest.Lock.Workloads[0].SHA256 = strings.Repeat("b", 64)
			case "oracle":
				b.bundle.Manifest.Lock.Workloads[0].Oracle.Expected = []uint64{2}
			case "function":
				b.bundle.Trials[0].CodeImage.Functions = b.bundle.Trials[0].CodeImage.Functions[1:]
				b.report.Records[0] = nativeRecord(b.bundle.Trials[0], 0)
			case "legacy":
				image := b.bundle.Trials[0].CodeImage
				image.Version = 1
				image.FunctionAttribution = "unavailable"
				image.Functions = nil
				b.report.Records[0] = nativeRecord(b.bundle.Trials[0], 0)
			case "block":
				b.bundle.Trials[0].Block = 1
				b.report.Records[0] = nativeRecord(b.bundle.Trials[0], 0)
			}
			x, err := compareNativeInputs(a, b, "a", "b")
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "artifact", "oracle":
				if x.Results[0].Status != "incomparable" || len(x.Results[0].Functions) != 0 {
					t.Fatal("compared different tasks", x)
				}
			case "function":
				if x.Results[0].Status != "partial" || len(x.Results[0].Functions) != 2 || x.Results[0].Functions[1].Candidate != nil {
					t.Fatal("lost missing function", x)
				}
			case "legacy":
				if x.Results[0].Status != "unavailable" || len(x.Results[0].Functions) != 2 || x.Results[0].Functions[0].SizeDelta != nil {
					t.Fatal("fabricated legacy attribution", x)
				}
			case "block":
				if len(x.Results) != 2 || x.Results[0].CandidateRecord != nil || x.Results[1].BaselineRecord != nil {
					t.Fatal("lost independent launch coverage", x)
				}
			}
		})
	}
}

func TestNativeComparisonRejectsHostProtocolAndDuplicateCells(t *testing.T) {
	for _, mode := range []string{"host", "protocol", "profile", "runtime", "duplicate", "range", "irq", "partition", "baseline"} {
		t.Run(mode, func(t *testing.T) {
			a, b := nativeComparisonFixture()
			candidate := "b"
			switch mode {
			case "irq":
				b.bundle.Manifest.Lock.RequireIRQAffinity = true
			case "partition":
				b.bundle.Manifest.Lock.RequireIsolatedCPUPartition = true
			case "baseline":
				b.bundle.Manifest.Lock.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion}
			case "host":
				b.bundle.Manifest.Host.Arch = "other"
			case "protocol":
				b.bundle.Manifest.Lock.Protocol = 999
			case "profile":
				b.bundle.Manifest.Lock.Options.Profile = "timing"
			case "runtime":
				candidate = "absent"
			case "duplicate":
				b.bundle.Trials = append(b.bundle.Trials, b.bundle.Trials[0])
				b.report.Records = append(b.report.Records, b.report.Records[0])
			case "range":
				b.bundle.Trials[0].CodeImage.Functions[0].Length = 999
			}
			if _, err := compareNativeInputs(a, b, "a", candidate); err == nil {
				t.Fatal("incompatible evidence accepted")
			}
		})
	}
}

func TestNativeComparisonPageEscapesAssemblyAndQuotesCLIIdentifiers(t *testing.T) {
	a, b := nativeComparisonFixture()
	x, err := compareNativeInputs(a, b, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	x.Results[0].Functions[0].Baseline.Listing = "</pre><script>alert(1)</script>"
	x.BaselineConfiguration.ID = "a'$(touch bad)"
	page, err := renderNativeComparison(x)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "<script>alert(1)</script>") || !strings.Contains(string(page), "&lt;/pre&gt;") {
		t.Fatal("unescaped compiler diagnostic")
	}
	if !strings.Contains(string(page), "&#39;a&#39;\\&#39;&#39;$(touch bad)&#39;") {
		t.Fatal("unsafe shell identifier", string(page))
	}
}

func TestLiveNativeComparisonRejectsResealedChanges(t *testing.T) {
	root := os.Getenv("WASMBENCH_NATIVE_COMPARISON_REPORT")
	if root == "" {
		t.Skip("set WASMBENCH_NATIVE_COMPARISON_REPORT to a verified multi-runtime export")
	}
	out := filepath.Join(t.TempDir(), "comparison")
	if err := CompareNativeCode(root, root, "wasmtime", "wasmtime-winch", out); err != nil {
		t.Fatal(err)
	}
	var x NativeCodeComparison
	if err := experiment.ReadJSON(filepath.Join(out, "data.json"), &x); err != nil {
		t.Fatal(err)
	}
	if len(x.Results) != 5 {
		t.Fatal("live corpus coverage changed", x)
	}
	original, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"status", "delta", "listing", "page"} {
		var changed NativeCodeComparison
		if err = json.Unmarshal(original, &changed); err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "status":
			changed.Results[0].Functions[0].Status = "identical_bytes"
		case "delta":
			delta := int64(123456)
			changed.Results[0].Functions[0].SizeDelta = &delta
		case "listing":
			changed.Results[0].Functions[0].Baseline.Listing = "forged"
		case "page":
			if err = os.WriteFile(filepath.Join(out, "index.html"), []byte("forged"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		data, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "data.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(filepath.Join(out, "checksums.json")); err != nil {
			t.Fatal(err)
		}
		if err = experiment.Seal(out); err != nil {
			t.Fatal(err)
		}
		if VerifyNativeComparison(out) == nil {
			t.Fatalf("resealed %s forgery accepted", mode)
		}
	}
	if err := CompareNativeCode(root, root, "wasmtime", "wasmtime-winch", out); err == nil {
		t.Fatal("existing report overwritten")
	}
	if err := CompareNativeCode(root, root, "wasmtime", "wasmtime-winch", filepath.Join(root, "nested")); err == nil {
		t.Fatal("source evidence overwritten")
	}
}
