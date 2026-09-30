package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func traceFixture(data string) EngineTrace {
	x := EngineTrace{Version: 1, ModuleSHA256: strings.Repeat("a", 64), Format: "chrome-trace-event-json", Collector: "node:inspector/NodeTracing", CollectorVersion: "test-v8", EmbeddingVersion: "test-node", Scope: "adapter_process_v8_wasm_events_without_module_attribution", Window: "run_request_including_setup_warmup_verification_and_trace_flush", Quality: "engine_reported", Categories: []string{"v8.wasm"}, Status: "collected", StartClockNS: "9007199254740993", EndClockNS: "9007199254741993", TrajectoryEpochNS: "9007199254741000", Data: []byte(data)}
	sum := sha256.Sum256(x.Data)
	x.SHA256 = hex.EncodeToString(sum[:])
	return x
}

func TestEngineTraceNativeEvidence(t *testing.T) {
	data := `{"traceEvents":[{"pid":1,"tid":2,"ts":10,"dur":0,"ph":"X","cat":"v8.wasm","name":"wasm.SyncCompile","args":{"id":0},"native_future_field":true},{"pid":1,"tid":2,"ts":0,"ph":"M","cat":"__metadata","name":"thread_name","args":{"name":"JavaScriptMainThread"}}]}`
	x := traceFixture(data)
	if err := x.Validate(x.ModuleSHA256); err != nil {
		t.Fatal(err)
	}
	events, err := x.Events()
	if err != nil || len(events) != 2 || events[0].Duration.String() != "0" {
		t.Fatal(events, err)
	}
	if string(x.Data) != data {
		t.Fatal("native bytes altered")
	}
	for name, change := range map[string]func(*EngineTrace){
		"hash":            func(x *EngineTrace) { x.SHA256 = strings.Repeat("b", 64) },
		"module":          func(x *EngineTrace) { x.ModuleSHA256 = strings.Repeat("b", 64) },
		"scope":           func(x *EngineTrace) { x.Scope = "guest_function_compile" },
		"category":        func(x *EngineTrace) { x.Categories = append(x.Categories, "other") },
		"clock_precision": func(x *EngineTrace) { x.StartClockNS = "9.007199254740993e15" },
		"reversed":        func(x *EngineTrace) { x.EndClockNS = "1" },
		"epoch":           func(x *EngineTrace) { x.TrajectoryEpochNS = "1" },
		"status":          func(x *EngineTrace) { x.Status = "complete_codegen" },
		"reason":          func(x *EngineTrace) { x.Reason = "contradictory" },
	} {
		t.Run(name, func(t *testing.T) {
			y := traceFixture(data)
			change(&y)
			if y.Validate(x.ModuleSHA256) == nil {
				t.Fatal("accepted forged trace")
			}
		})
	}
	for _, bad := range []string{`{}`, `{"traceEvents":null}`, `{"traceEvents":[{"pid":1,"tid":2,"ts":10,"ph":"X","cat":"v8.wasm","name":"compile"}]}`, `{"traceEvents":[{"pid":1,"tid":2,"ts":-1,"ph":"M","cat":"__metadata","name":"name"}]}`} {
		y := traceFixture(bad)
		if y.Validate(y.ModuleSHA256) == nil {
			t.Fatal("accepted malformed events", bad)
		}
	}
	x = traceFixture(`{"traceEvents":[]}`)
	if err := x.Validate(x.ModuleSHA256); err != nil {
		t.Fatal("empty collection is valid", err)
	}
	x.Status = "incomplete"
	x.Reason = "retained prefix"
	if err := x.Validate(x.ModuleSHA256); err != nil {
		t.Fatal(err)
	}
	x.Status = "unavailable"
	if x.Validate(x.ModuleSHA256) == nil {
		t.Fatal("unavailable retained fake data")
	}
	x.Data = nil
	x.SHA256 = ""
	x.StartClockNS = ""
	x.EndClockNS = ""
	x.TrajectoryEpochNS = ""
	if err := x.Validate(x.ModuleSHA256); err != nil {
		t.Fatal(err)
	}
}

func TestEngineTraceRunBounded(t *testing.T) {
	w, _ := tierFixture()
	p := Preparation{Workload: w, Profile: "profiling"}
	r := RunRequest{Scenario: "trajectory", Samples: 9998, Warmup: 2, Operations: 1}
	if err := ValidateEngineTraceRun(&p, &r); err != nil {
		t.Fatal(err)
	}
	r.Samples++
	if ValidateEngineTraceRun(&p, &r) == nil {
		t.Fatal("unbounded trace packet")
	}
	if err := ValidateTierRun(&p, &r); err != nil {
		t.Fatal("plain tiers unnecessarily restricted", err)
	}
}
