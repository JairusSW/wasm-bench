package publish

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func traceExportFixture() experiment.Bundle {
	module := strings.Repeat("a", 64)
	data := []byte(`{"traceEvents":[{"pid":1,"tid":2,"ts":0,"ph":"X","cat":"v8.wasm","name":"wasm.SyncCompile","dur":0,"tdur":0,"args":null,"future_field":"kept"},{"pid":1,"tid":2,"ts":0,"ph":"M","cat":"__metadata","name":"thread_name","args":{"name":"JavaScriptMainThread"}},{"pid":1,"tid":2,"ts":0,"ph":"M","cat":"__metadata","name":"thread_name","args":{"name":"JavaScriptMainThread"}}]}`)
	x := &protocol.EngineTrace{Version: 1, ModuleSHA256: module, Format: "chrome-trace-event-json", Collector: "node:inspector/NodeTracing", CollectorVersion: "test-v8", EmbeddingVersion: "test-node", Scope: "adapter_process_v8_wasm_events_without_module_attribution", Window: "run_request_including_setup_warmup_verification_and_trace_flush", Quality: "engine_reported", Categories: []string{"v8.wasm"}, Status: "incomplete", Reason: "retained prefix", StartClockNS: "18446744073709551605", EndClockNS: "18446744073709551615", TrajectoryEpochNS: "18446744073709551610", Data: data, SHA256: corpus.Hash(data)}
	missing := *x
	missing.Status = "unavailable"
	missing.Reason = "start failed"
	missing.StartClockNS = ""
	missing.EndClockNS = ""
	missing.TrajectoryEpochNS = ""
	missing.Data = nil
	missing.SHA256 = ""
	empty := *x
	empty.Status = "collected"
	empty.Reason = ""
	empty.Data = []byte(`{"traceEvents":[]}`)
	empty.SHA256 = corpus.Hash(empty.Data)
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: "run", Lock: experiment.Lock{Options: experiment.Options{Profile: "profiling", Samples: 3, Warmup: 2, Operations: 1}, Workloads: []protocol.Workload{{ID: "work", SHA256: module, ABI: "core", Reset: "stateless", Export: "run", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}}, Runtimes: []experiment.Runtime{{ID: "traced", Description: &protocol.Description{Version: "test-v8", Build: "test-node", Capabilities: map[string]bool{"can_trace_v8_wasm_events": true, "can_profile_tier_trajectory": true}}}, {ID: "other", Description: &protocol.Description{Capabilities: map[string]bool{}}}}}}}
	base := experiment.Trial{Runtime: "traced", Workload: "work", Scenario: "trajectory", Profile: "profiling", Status: "error", Reason: "later guest failure"}
	for i, x := range []*protocol.EngineTrace{x, &missing, &empty, nil} {
		trial := base
		trial.ID = string(rune('a' + i))
		trial.EngineTrace = x
		b.Trials = append(b.Trials, trial)
	}
	admission := base
	admission.ID = "admission"
	admission.Block = -1
	admission.Status = "ok"
	admission.Scenario = "first-call"
	unsupported := base
	unsupported.ID = "unsupported"
	unsupported.Runtime = "other"
	unsupported.Status = "unsupported"
	unsupported.Reason = "contract unavailable"
	timing := base
	timing.ID = "timing"
	timing.Profile = "timing"
	timing.Runtime = "other"
	b.Trials = append(b.Trials, admission, unsupported, timing)
	return b
}

func TestEngineTraceParquetExactNullableNativeRows(t *testing.T) {
	b := traceExportFixture()
	root := t.TempDir()
	path := filepath.Join(root, "engine-events.parquet")
	if err := ExportEngineTraces(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[EngineTraceRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 9 {
		t.Fatal("lost coverage or duplicate events", len(rows))
	}
	r := rows[0]
	if r.RowKind != "trial_outcome" || r.TraceStatus != "incomplete" || r.TraceReason != "retained prefix" || r.TrialStatus != "error" || r.TrialReason != "later guest failure" || r.ExportVersion != EngineTraceExportVersion || *r.EventCount != 3 || r.EventIndex != nil {
		t.Fatal(r)
	}
	if *r.EndClockNS != math.MaxUint64 || *r.StartClockNS != math.MaxUint64-10 || *r.TrajectoryEpochNS != math.MaxUint64-5 {
		t.Fatal("rounded exact collection clocks", r)
	}
	e := rows[1]
	if e.EventCount != nil {
		t.Fatal("repeated trial count on native event row")
	}
	if *e.EventIndex != 0 || *e.TimestampUS != 0 || e.DurationUS == nil || *e.DurationUS != 0 || e.ThreadDurationUS == nil || *e.ThreadDurationUS != 0 || *e.ArgsJSON != "null" || !strings.Contains(*e.NativeEventJSON, `"future_field":"kept"`) {
		t.Fatal("zero or native fields lost", e)
	}
	if rows[2].DurationUS != nil || rows[2].ThreadDurationUS != nil || *rows[2].NativeEventJSON != *rows[3].NativeEventJSON || *rows[3].EventIndex != 2 {
		t.Fatal("missing duration invented or duplicate metadata lost")
	}
	if rows[4].TraceStatus != "unavailable" || rows[4].EventCount != nil || rows[4].StartClockNS != nil || rows[4].TimestampUS != nil {
		t.Fatal("unavailable turned into zero", rows[4])
	}
	if rows[5].TraceStatus != "collected" || rows[5].EventCount == nil || *rows[5].EventCount != 0 || rows[5].EventIndex != nil {
		t.Fatal("empty collection not explicit", rows[5])
	}
	if rows[6].TraceStatus != "not_recorded" || rows[7].TraceStatus != "not_applicable" || rows[8].TraceStatus != "unsupported" {
		t.Fatal("coverage omitted", rows[6:])
	}
	if err := verifyEngineTraceParquet(b, root); err != nil {
		t.Fatal("writer not deterministic", err)
	}
	if ExportEngineTraces(b, path) == nil {
		t.Fatal("overwrote export")
	}
	if err := os.WriteFile(path, []byte("forged"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifyEngineTraceParquet(b, root) == nil {
		t.Fatal("forged typed download accepted")
	}
}

func TestEngineTraceParquetRejectsMalformedAndEmptyTypedFile(t *testing.T) {
	b := traceExportFixture()
	b.Trials[0].EngineTrace.SHA256 = "forged"
	path := filepath.Join(t.TempDir(), "bad.parquet")
	if ExportEngineTraces(b, path) == nil {
		t.Fatal("accepted malformed evidence")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("created output before validation")
	}
	path = filepath.Join(t.TempDir(), "empty.parquet")
	if err := ExportEngineTraces(experiment.Bundle{}, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[EngineTraceRow](path)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	// The native-event JSON column is itself parseable, not a typed reconstruction.
	b = traceExportFixture()
	var raw struct {
		Events []json.RawMessage `json:"traceEvents"`
	}
	if err := json.Unmarshal(b.Trials[0].EngineTrace.Data, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Events) != 3 {
		t.Fatal("fixture malformed")
	}
}
