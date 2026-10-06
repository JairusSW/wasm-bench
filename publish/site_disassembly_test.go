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
	if err := DisassembleNativeCode(context.Background(), raw, code, copyPath, dumpPath, 10*time.Second); err != nil {
		t.Fatal(err)
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
	if err = writeSiteDatasetFiles(d, data, seal, out, report, bundle); err != nil {
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
