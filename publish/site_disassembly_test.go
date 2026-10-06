package publish

import (
	"context"
	"encoding/json"
	"github.com/wasmbench/wasmbench/experiment"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSiteDisassemblyBoundsAndIdentity(t *testing.T) {
	_, bundle, _ := nativeSiteFixture()
	image := *bundle.Trials[0].CodeImage
	listing := strings.Repeat("0: nop\n", 10000) + "trailing"
	source := NativeExportRecord{Image: &image, Disassembly: &NativeDisassembly{Functions: []FunctionDisassembly{{Function: image.Functions[0], Listing: listing, ObjdumpArgs: []string{"--disassemble", "--disassemble-zeroes", "--section=.text", "--start-address=0", "--stop-address=32", "image.o"}}}}}
	tools := []NativeTool{{SHA256: siteHash([]byte("copy")), Version: "fixture"}, {SHA256: siteHash([]byte("dump")), Version: "fixture"}}
	objects := map[string][]byte{}
	object := func(kind string, v any) (string, error) {
		b, err := siteJSON(v)
		if err != nil {
			return "", err
		}
		if len(b) > SiteChunkBytes {
			t.Fatal("unbounded disassembly object", len(b))
		}
		id := siteHash(b)
		objects[id] = b
		return id, nil
	}
	refs, counts, err := siteNativeFunctions(image, &source, tools, "native-image-disassembly-v2", object)
	if err != nil || len(refs) != 1 || counts[0] != 1 {
		t.Fatal(refs, counts, err)
	}
	var shard struct {
		Functions  []siteNativeFunction `json:"functions"`
		References []string             `json:"references"`
	}
	if err = json.Unmarshal(objects[refs[0]], &shard); err != nil {
		t.Fatal(err)
	}
	if len(shard.References) != 1 || shard.Functions[0].Disassembly != shard.References[0] {
		t.Fatal(shard)
	}
	var derivative struct {
		TextSHA256 string   `json:"textSha256"`
		Bytes      int      `json:"bytes"`
		References []string `json:"references"`
		Lines      int      `json:"lines"`
	}
	_ = json.Unmarshal(objects[shard.References[0]], &derivative)
	joined := ""
	for _, ref := range derivative.References {
		var chunk struct {
			Lines []string `json:"lines"`
		}
		_ = json.Unmarshal(objects[ref], &chunk)
		if len(chunk.Lines) > 256 {
			t.Fatal("unbounded lines")
		}
		joined += strings.Join(chunk.Lines, "")
	}
	if joined != listing || derivative.TextSHA256 != siteHash([]byte(listing)) || derivative.Bytes != len(listing) || derivative.Lines != 10001 {
		t.Fatal("diagnostic text changed")
	}
	source.Disassembly.Functions[0].Function.Offset++
	if _, _, err = siteNativeFunctions(image, &source, tools, "native-image-disassembly-v2", object); err == nil {
		t.Fatal("forged mapping")
	}
}

// Actual LLVM, synthetic attributed bytes: this is diagnostic transport
// qualification, not a benchmark execution or a real runtime result claim.
func TestSiteExportSealedLLVMDisassembly(t *testing.T) {
	if os.Getenv("WASMBENCH_NATIVE_LLVM_TEST") != "1" {
		t.Skip("opt-in installed LLVM diagnostic gate")
	}
	raw := nativeBundle(t, "code")
	var trial experiment.Trial
	if err := experiment.ReadJSON(filepath.Join(raw, "trials", "a.json"), &trial); err != nil {
		t.Fatal(err)
	}
	d, bundle, _ := nativeSiteFixture()
	trial = bundle.Trials[0]
	var manifest experiment.Manifest
	if err := experiment.ReadJSON(filepath.Join(raw, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ID = bundle.Manifest.ID
	manifest.Lock.Workloads = d.Bundle.Manifest.Lock.Workloads
	manifestJSON, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(raw, "manifest.json"), manifestJSON, 0644); err != nil {
		t.Fatal(err)
	}
	trialJSON, _ := json.Marshal(trial)
	if err := os.WriteFile(filepath.Join(raw, "trials", "a.json"), trialJSON, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(raw, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(raw); err != nil {
		t.Fatal(err)
	}
	report := t.TempDir()
	code := filepath.Join(report, "code")
	copyPath, dumpPath := os.Getenv("WASMBENCH_LLVM_OBJCOPY"), os.Getenv("WASMBENCH_LLVM_OBJDUMP")
	if copyPath == "" {
		copyPath = "/opt/homebrew/opt/llvm/bin/llvm-objcopy"
	}
	if dumpPath == "" {
		dumpPath = "/opt/homebrew/opt/llvm/bin/llvm-objdump"
	}
	generate := DisassembleNativeCode
	if os.Getenv("WASMBENCH_FUNCTIONS_ONLY_TEST") == "1" {
		generate = DisassembleNativeFunctions
	}
	if err := generate(context.Background(), raw, code, copyPath, dumpPath, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	regenerated := filepath.Join(t.TempDir(), "regenerated")
	if err := regenerateNativeDisassembly(code, regenerated); err != nil {
		t.Fatal("offline derivative regeneration", err)
	}
	if err := VerifyNativeCode(regenerated); err != nil {
		t.Fatal("regenerated derivative verification", err)
	}
	if os.Getenv("WASMBENCH_FUNCTIONS_ONLY_TEST") == "1" {
		var native NativeExport
		if err := experiment.ReadJSON(filepath.Join(code, "native-code.json"), &native); err != nil {
			t.Fatal(err)
		}
		if native.Version != "native-image-disassembly-v3" || native.FunctionListingLimitBytes != 128<<20 {
			t.Fatal("function-only policy not recorded")
		}
		for _, record := range native.Records {
			if record.Disassembly != nil && (record.Disassembly.Text != "" || record.Disassembly.ObjdumpLog != "" || record.Disassembly.Status != "function_ranges") {
				t.Fatal("whole-image diagnostic inferred")
			}
		}
		html, err := os.ReadFile(filepath.Join(code, "index.html"))
		if err != nil || strings.Contains(string(html), "download>Linear disassembly</a>") {
			t.Fatal("missing full listing download advertised", err)
		}
	}
	data, _ := json.Marshal(d)
	if err := os.WriteFile(filepath.Join(report, "data.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(report); err != nil {
		t.Fatal(err)
	}
	seal, err := os.ReadFile(filepath.Join(report, "checksums.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "site")
	external := ""
	if os.Getenv("WASMBENCH_EXTERNAL_DISASSEMBLY_TEST") == "1" {
		external = filepath.Join(t.TempDir(), "native")
		if err = os.Mkdir(external, 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.CopyFS(external, os.DirFS(code)); err != nil {
			t.Fatal(err)
		}
	}
	if err = writeSiteDatasetSources(d, data, seal, out, report, external, bundle); err != nil {
		t.Fatal(err)
	}
	var export SiteManifest
	if err = experiment.ReadJSON(filepath.Join(out, "manifest.json"), &export); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range export.Objects {
		if o.Kind != "record" {
			continue
		}
		var r SiteRecord
		if err = experiment.ReadJSON(filepath.Join(out, "objects", o.SHA256), &r); err != nil {
			t.Fatal(err)
		}
		if r.Kind == "artifact" {
			var a struct {
				Inspection struct {
					Disassembly struct {
						Status string `json:"status"`
					} `json:"disassembly"`
				} `json:"inspection"`
			}
			_ = json.Unmarshal(r.Data, &a)
			if a.Inspection.Disassembly.Status != "available" {
				t.Fatal("verified listings omitted")
			}
			found = true
		}
	}
	records, _, _, scope, err := siteDisassemblies(report, external, d.CodeSource)
	if err != nil || len(records) == 0 || scope.CodeSealSHA256 == "" || scope.NativeSealSHA256 == "" || scope.Verification != "producer-asserted" {
		t.Fatal("archive identity absent", scope, err)
	}
	wrong := *d.CodeSource
	wrong.ID = "wrong-pass"
	if _, _, _, _, err = siteDisassemblies(report, external, &wrong); err == nil {
		t.Fatal("foreign pass accepted")
	}
	if external != "" {
		// A separately valid archive with the same run ID but different sealed
		// source metadata must not substitute for the report's exact pass.
		foreignRaw := filepath.Join(t.TempDir(), "raw")
		if err = os.Mkdir(foreignRaw, 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.CopyFS(foreignRaw, os.DirFS(raw)); err != nil {
			t.Fatal(err)
		}
		var changed experiment.Manifest
		if err = experiment.ReadJSON(filepath.Join(foreignRaw, "manifest.json"), &changed); err != nil {
			t.Fatal(err)
		}
		changed.Created = time.Date(2026, 10, 6, 1, 2, 3, 0, time.UTC)
		bytes, _ := json.Marshal(changed)
		if err = os.WriteFile(filepath.Join(foreignRaw, "manifest.json"), bytes, 0644); err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(filepath.Join(foreignRaw, "checksums.json")); err != nil {
			t.Fatal(err)
		}
		if err = experiment.Seal(foreignRaw); err != nil {
			t.Fatal(err)
		}
		foreign := filepath.Join(t.TempDir(), "native")
		if err = generate(context.Background(), foreignRaw, foreign, copyPath, dumpPath, 10*time.Second); err != nil {
			t.Fatal(err)
		}
		if err = VerifyNativeCode(foreign); err != nil {
			t.Fatal("foreign archive fixture must be valid", err)
		}
		if _, _, _, _, err = siteDisassemblies(report, foreign, d.CodeSource); err == nil || !strings.Contains(err.Error(), "source seal differs") {
			t.Fatal("same-ID foreign sealed pass accepted", err)
		}
	}
	if external != "" && scope.Location != "external-archive" {
		t.Fatal("external archive identity hidden")
	}
	if !found {
		t.Fatal("missing artifact")
	}
	if destination := os.Getenv("WASMFYI_DISASSEMBLY_FIXTURE_OUT"); destination != "" {
		if err = os.Mkdir(destination, 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.CopyFS(destination, os.DirFS(out)); err != nil {
			t.Fatal(err)
		}
	}
}
