package publish

import (
	"bytes"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestEngineTraceDownloadsNativeIncompleteAndNewOnly(t *testing.T) {
	data := []byte(`{"traceEvents":[]}`)
	digest := corpus.Hash(data)
	x := &protocol.EngineTrace{Version: 1, ModuleSHA256: digest, Format: "chrome-trace-event-json", Collector: "node:inspector/NodeTracing", CollectorVersion: "test", EmbeddingVersion: "test", Scope: "adapter_process_v8_wasm_events_without_module_attribution", Window: "run_request_including_setup_warmup_verification_and_trace_flush", Quality: "engine_reported", Categories: []string{"v8.wasm"}, Status: "incomplete", Reason: "prefix", StartClockNS: "1", EndClockNS: "2", Data: data, SHA256: digest}
	b := experiment.Bundle{Trials: []experiment.Trial{{EngineTrace: x}, {EngineTrace: x}, {Status: "unsupported"}}}
	root := t.TempDir()
	if err := exportEngineTraces(b, root); err != nil {
		t.Fatal(err)
	}
	if err := verifyEngineTraceExports(b, root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "traces", digest+".trace.json"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal(err)
	}
	if exportEngineTraces(b, root) == nil {
		t.Fatal("overwrote download")
	}
	if err := os.WriteFile(filepath.Join(root, "traces", digest+".trace.json"), []byte("forged"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifyEngineTraceExports(b, root) == nil {
		t.Fatal("forged download accepted")
	}
	x.SHA256 = "../../escape"
	if exportEngineTraces(b, t.TempDir()) == nil {
		t.Fatal("unsafe digest accepted")
	}
}
