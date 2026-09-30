package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func nativeBundle(t *testing.T, profile string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	digest := corpus.Hash([]byte{0, 1, 255})
	m := experiment.Manifest{ID: "native", Lock: experiment.Lock{Options: experiment.Options{Profile: profile}, Workloads: []protocol.Workload{{ID: "w", SHA256: digest}}}}
	if err := experiment.WriteJSON(filepath.Join(root, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	trials := []experiment.Trial{
		{ID: "../../outside", Runtime: "r", Workload: "w", Profile: "code", Status: "ok", CodeImage: &protocol.CodeImage{Version: 1, ModuleSHA256: digest, SHA256: digest, Architecture: "arm64", Backend: "railshot", Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "unavailable", Data: []byte{0, 1, 255}}},
		{ID: "missing", Runtime: "interpreter", Workload: "w", Profile: "code", Status: "ok"},
		{ID: "failure", Runtime: "r", Workload: "w", Profile: "code", Status: "timeout", Reason: "deadline"},
		{ID: "check", Runtime: "r", Workload: "w", Profile: "code", Status: "ok", Block: -1},
		{ID: "oversize", Runtime: "r", Workload: "w", Profile: "code", Status: "ok", Observations: []protocol.Observation{{Metric: "native.code_export", Status: "unavailable", Reason: "size limit"}}},
	}
	for i, trial := range trials {
		if err := experiment.WriteJSON(filepath.Join(root, "trials", string(rune('a'+i))+".json"), trial); err != nil {
			t.Fatal(err)
		}
	}
	if err := experiment.Seal(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOfflineNativeExport(t *testing.T) {
	root := nativeBundle(t, "code")
	out := filepath.Join(t.TempDir(), "export")
	if err := ExportNativeCode(root, out); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Verify(root); err != nil {
		t.Fatal("input modified", err)
	}
	if err := experiment.Verify(out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyNativeCode(out); err != nil {
		t.Fatal(err)
	}
	var r NativeExport
	if err := experiment.ReadJSON(filepath.Join(out, "native-code.json"), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Records) != 5 {
		t.Fatal(r)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Native output evidence", "mixed_code_and_embedded_data", "image-000000.bin", "size limit", "Not available", "raw/manifest.json"} {
		if !strings.Contains(string(html), fragment) {
			t.Fatalf("code report missing %q", fragment)
		}
	}
	if strings.Contains(string(html), `href="../../outside`) {
		t.Fatal("untrusted trial ID became a link")
	}
	for i, want := range []string{"available", "not_recorded", "unavailable", "not_collected", "unavailable"} {
		if r.Records[i].Status != want {
			t.Fatal(r.Records[i])
		}
		if i > 0 && (r.Records[i].Bytes != nil || r.Records[i].Path != "") {
			t.Fatal("fabricated missing export", r.Records[i])
		}
	}
	first := r.Records[0]
	if first.Image == nil || first.Image.Data != nil || first.Bytes == nil || *first.Bytes != 3 {
		t.Fatal(first)
	}
	data, err := os.ReadFile(filepath.Join(out, first.Path))
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Hash(data) != first.Image.SHA256 {
		t.Fatal("bytes changed")
	}
	info, err := os.Stat(filepath.Join(out, first.Path))
	if err != nil || info.Mode().Perm()&0111 != 0 {
		t.Fatal("image executable", err)
	}
	if r.Records[2].Reason != "deadline" || r.Records[4].Reason != "size limit" {
		t.Fatal("lost failure reasons")
	}
	if err := ExportNativeCode(root, out); err == nil {
		t.Fatal("overwrote export")
	}
	if err := experiment.Verify(out); err != nil {
		t.Fatal("existing export modified", err)
	}
}

func TestAttributedNativeExportPreservesRanges(t *testing.T) {
	replaceJSON := func(path string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	root := nativeBundle(t, "code")
	path := filepath.Join(root, "trials", "a.json")
	var trial experiment.Trial
	if err := experiment.ReadJSON(path, &trial); err != nil {
		t.Fatal(err)
	}
	trial.CodeImage.Version = 2
	trial.CodeImage.Backend = "winch"
	trial.CodeImage.FunctionAttribution = "engine_reported"
	trial.CodeImage.Functions = []protocol.CodeFunction{{WasmIndex: 7, Name: "<guest>", Offset: 1, Length: 2, Tier: "winch"}}
	replaceJSON(path, trial)
	if err := os.Remove(filepath.Join(root, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(root); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "export")
	if err := ExportNativeCode(root, out); err != nil {
		t.Fatal(err)
	}
	var exported NativeExport
	if err := experiment.ReadJSON(filepath.Join(out, "native-code.json"), &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported.Records[0].Image.Functions) != 1 || exported.Records[0].Image.Functions[0].WasmIndex != 7 {
		t.Fatal("lost ranges")
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "&lt;guest&gt;") || !strings.Contains(string(page), "engine-reported function ranges") {
		t.Fatal("missing escaped range table")
	}
	// A newly sealed but fabricated function mapping is not source evidence.
	exported.Records[0].Image.Functions[0].WasmIndex = 8
	replaceJSON(filepath.Join(out, "native-code.json"), exported)
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if VerifyNativeCode(out) == nil {
		t.Fatal("accepted fabricated function mapping")
	}
}

func TestFunctionAssemblyIsEscapedInPage(t *testing.T) {
	page, err := renderCodeHTML(NativeExport{Records: []NativeExportRecord{{Runtime: "r", Image: &protocol.CodeImage{Version: 2, Functions: []protocol.CodeFunction{{WasmIndex: 7}}}, Disassembly: &NativeDisassembly{Functions: []FunctionDisassembly{{Listing: "</pre><script>alert(1)</script>", Text: "listing.asm"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "<script>alert(1)</script>") || !strings.Contains(string(page), "&lt;/pre&gt;&lt;script&gt;") {
		t.Fatal("unescaped compiler diagnostic")
	}
	if !strings.Contains(string(page), "View function 7 assembly") {
		t.Fatal("missing drilldown")
	}
}

func TestVerifyNativeCodeRejectsReSealedForgery(t *testing.T) {
	for _, change := range []string{"page", "image", "record", "interpretation", "renderer"} {
		t.Run(change, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "export")
			if err := ExportNativeCode(nativeBundle(t, "code"), out); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "page":
				if err := os.WriteFile(filepath.Join(out, "index.html"), []byte("forged page"), 0644); err != nil {
					t.Fatal(err)
				}
			case "image":
				if err := os.WriteFile(filepath.Join(out, "image-000000.bin"), []byte("different"), 0644); err != nil {
					t.Fatal(err)
				}
			case "record", "interpretation", "renderer":
				var r NativeExport
				if err := experiment.ReadJSON(filepath.Join(out, "native-code.json"), &r); err != nil {
					t.Fatal(err)
				}
				if change == "record" {
					r.Records[0].Bytes = protocolValue(999)
				} else {
					if change == "interpretation" {
						r.Interpretation = "all bytes are guest instructions"
					} else {
						r.RendererSHA256 = strings.Repeat("0", 64)
					}
				}
				if err := os.Remove(filepath.Join(out, "native-code.json")); err != nil {
					t.Fatal(err)
				}
				if err := experiment.WriteJSON(filepath.Join(out, "native-code.json"), r); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
				t.Fatal(err)
			}
			if err := experiment.Seal(out); err != nil {
				t.Fatal(err)
			}
			if err := VerifyNativeCode(out); err == nil {
				t.Fatal("accepted re-sealed forgery")
			}
		})
	}
}

func protocolValue(n int) *int { return &n }

func TestNativeCodePageEscapesUntrustedEvidence(t *testing.T) {
	r := NativeExport{Run: `<img src=x onerror=alert(1)>`, Interpretation: `<script>alert(1)</script>`, Records: []NativeExportRecord{{Runtime: `<svg/onload=alert(1)>`, Status: "unavailable", Reason: `<b>wrong</b>`}}}
	html, err := renderCodeHTML(r)
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, raw := range []string{`<img src=x`, `<script>alert`, `<svg/onload`, `<b>wrong</b>`} {
		if strings.Contains(page, raw) {
			t.Fatalf("unescaped evidence %q", raw)
		}
	}
}

func TestNativeRecordRetainsExplicitUnsupportedCodeExport(t *testing.T) {
	trial := experiment.Trial{ID: "one", Runtime: "r", Workload: "w", Profile: "code", Scenario: "compile", Status: "ok", Observations: []protocol.Observation{{Metric: "native.guest_code", Status: "unsupported", Reason: "API has no code export"}}}
	got := nativeRecord(trial, 0)
	if got.Status != "unsupported" || got.Reason != "API has no code export" || got.Bytes != nil {
		t.Fatal(got)
	}
	trial.Observations = append(trial.Observations, protocol.Observation{Metric: "native.code_export", Status: "unavailable", Reason: "size limit"})
	got = nativeRecord(trial, 0)
	if got.Status != "unavailable" || got.Reason != "size limit" {
		t.Fatal("specific whole-image failure must win", got)
	}
}

func TestNativeExportRejectsNestedAndWrongProfile(t *testing.T) {
	root := nativeBundle(t, "code")
	for _, base := range []string{root, filepath.Join(t.TempDir(), "alias")} {
		if base != root {
			if err := os.Symlink(root, base); err != nil {
				t.Fatal(err)
			}
		}
		if err := ExportNativeCode(root, filepath.Join(base, "nested")); err == nil {
			t.Fatal("nested output accepted")
		}
	}
	if err := experiment.Verify(root); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "wrong-profile")
	if err := ExportNativeCode(nativeBundle(t, "timing"), out); err == nil {
		t.Fatal("wrong profile accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("created invalid output", err)
	}
}
