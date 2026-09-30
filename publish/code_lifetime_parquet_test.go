package publish

import (
	"encoding/json"
	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestCodeLifetimeParquetRetainsExactEvidenceZeroAndMissing(t *testing.T) {
	bytes := []byte{1, 2, 3, 4}
	sha := corpus.Hash(bytes)
	image := &protocol.CodeImage{Version: 2, ModuleSHA256: sha, SHA256: sha, Architecture: "arm64", Backend: "cranelift", Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "engine_reported", Data: bytes, Functions: []protocol.CodeFunction{{WasmIndex: 0, Length: 4, Tier: "cranelift"}}}
	l := &protocol.CodeLifetime{Version: 1, Collector: "Wasmtime/CustomCodeMemory", CollectorVersion: "46.0.1", Scope: "published_executable_text_capacity", Quality: "engine_callback", ModuleSHA256: sha, ImageSHA256: sha, Backend: "cranelift", Architecture: "arm64", PageSize: 4096, Publication: 1, ReleasePolicy: "drop_module_handles_then_store_then_engine", BeforeDrop: protocol.Values{7}, AfterDrop: protocol.Values{7}, Events: []protocol.CodeLifetimeEvent{{Kind: "published", Publication: 1, ElapsedNS: 1, Address: "9007199254740992", Capacity: 4096, ActiveCapacity: 4096, CumulativeCapacity: 4096}, {Sequence: 1, Kind: "unpublished", Publication: 1, ElapsedNS: 5, Address: "9007199254740992", Capacity: 4096, CumulativeCapacity: 4096}}}
	for i, stage := range []string{"compiled", "instance_verified", "module_handles_dropped", "store_dropped", "engine_dropped"} {
		n := uint64(1)
		active := uint64(4096)
		if i >= 3 {
			n = 2
			active = 0
		}
		l.Checkpoints = append(l.Checkpoints, protocol.CodeLifetimeCheckpoint{Stage: stage, ElapsedNS: int64(i + 2), EventCount: n, ActiveCapacity: active, CumulativeCapacity: 4096})
	}
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: "run"}, Trials: []experiment.Trial{{ID: "good", Workload: "w", Runtime: "native", Profile: "code", Scenario: "code-lifetime", Status: "ok", CodeImage: image, CodeLifetime: l}, {ID: "missing", Profile: "code", Scenario: "code-lifetime", Status: "unsupported", Reason: "capability missing"}}}
	root := t.TempDir()
	path := filepath.Join(root, "code-lifetimes.parquet")
	if err := ExportCodeLifetimes(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[CodeLifetimeRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 9 {
		t.Fatalf("outcome+2 events+5 checkpoints+missing; got %d", len(rows))
	}
	if rows[2].Active == nil || *rows[2].Active != 0 || rows[2].Address == nil || *rows[2].Address != "9007199254740992" {
		t.Fatal("retirement zero/address precision lost")
	}
	if rows[8].Active != nil || rows[8].EvidenceJSON != nil || rows[8].Status != "unsupported" {
		t.Fatal("missing evidence turned into measured zero")
	}
	var decoded protocol.CodeLifetime
	if err := json.Unmarshal([]byte(*rows[0].EvidenceJSON), &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(sha, image); err != nil {
		t.Fatal(err)
	}
	if err := verifyCodeLifetimeParquet(b, root); err != nil {
		t.Fatal(err)
	}
	wrong := "rounded-address"
	rows[1].Address = &wrong
	if err := parquet.WriteFile(path, rows, reportParquetWriterOption()); err != nil {
		t.Fatal(err)
	}
	if verifyCodeLifetimeParquet(b, root) == nil {
		t.Fatal("derived export forgery accepted")
	}
	if ExportCodeLifetimes(b, path) == nil {
		t.Fatal("existing export overwritten")
	}
	empty := filepath.Join(t.TempDir(), "empty.parquet")
	if err := ExportCodeLifetimes(experiment.Bundle{}, empty); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); err != nil {
		t.Fatal(err)
	}
}
