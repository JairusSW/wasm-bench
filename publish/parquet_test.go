package publish

import (
	"encoding/json"
	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParquetSnapshotMemoryStaysDiagnostic(t *testing.T) {
	// Export fidelity only: sealed-loader tests enforce the native proof contract.
	ref := protocol.SnapshotProcess{PID: 101, StartTimeTicks: "9007199254740993"}
	e := &experiment.SnapshotMemoryEvidence{Version: experiment.SnapshotMemoryVersion, ControllerPID: 100,
		Diagnostics: protocol.SnapshotDiagnostics{Version: protocol.SnapshotInspectionVersion, Profile: "memory", Samples: []protocol.Sample{{ElapsedNS: 0, Operations: 1}}},
		Records:     []experiment.SnapshotMemoryRecord{{Boundary: protocol.SnapshotBoundary{Source: ref}, Readings: []collectors.SnapshotProcessReading{{Process: ref, StartNS: 0, EndNS: 1, StatBefore: "raw before", StatAfter: "raw after", Status: "VmRSS: 0 kB", SmapsStatus: "permission_denied", SmapsReason: "denied"}}}}}
	b := experiment.Bundle{Trials: []experiment.Trial{{Profile: "memory", Status: "ok", SnapshotMemory: e}, {Profile: "memory", Status: "unsupported"}}}
	p := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, p); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].SnapshotMemoryJSON == nil || rows[1].SnapshotMemoryJSON != nil || rows[0].ElapsedNS != nil || rows[0].SampleIndex != nil || rows[0].Operations != nil || rows[0].LatencyEligible || rows[0].ExportVersion != SampleExportVersion {
		t.Fatal("memory evidence promoted or lost", rows)
	}
	var actual experiment.SnapshotMemoryEvidence
	if err := json.Unmarshal([]byte(*rows[0].SnapshotMemoryJSON), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e, &actual) {
		t.Fatal("raw lineage, exact birth or missing smaps changed")
	}
}

func TestParquetSnapshotDensityStaysDiagnostic(t *testing.T) {
	zero := int64(0)
	e := &experiment.SnapshotDensityTrialEvidence{Version: experiment.SnapshotDensityTrialVersion, Groups: []experiment.SnapshotDensityEvidence{{Version: experiment.SnapshotDensityEvidenceVersion, Proof: protocol.SnapshotDensityProof{ProvisionElapsedNS: &zero, Source: protocol.SnapshotProcess{PID: 101, StartTimeTicks: "9007199254740993"}}, Records: []experiment.SnapshotDensityRecord{{Boundary: protocol.SnapshotDensityBoundary{SampleIndex: 1}, Readings: []collectors.SnapshotProcessReading{{SmapsStatus: "permission_denied", SmapsReason: "denied"}}}}}}}
	b := experiment.Bundle{Trials: []experiment.Trial{{Profile: "memory", Status: "ok", SnapshotDensity: e}, {Profile: "memory", Status: "unsupported"}}}
	path := filepath.Join(t.TempDir(), "density.parquet")
	if err := ExportParquet(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].SnapshotDensityJSON == nil || rows[1].SnapshotDensityJSON != nil || rows[0].ElapsedNS != nil || rows[0].SampleIndex != nil || rows[0].Operations != nil || rows[0].LatencyEligible {
		t.Fatal("density proof promoted or lost")
	}
	var actual experiment.SnapshotDensityTrialEvidence
	if err := json.Unmarshal([]byte(*rows[0].SnapshotDensityJSON), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e, &actual) {
		t.Fatal("exact births, group indexes, zeros or denied smaps changed")
	}
}

func TestRetainedSnapshotDensityParquet(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_DENSITY_PRODUCT_EVIDENCE_DIR")
	if root == "" {
		t.Skip("requires completed native density report")
	}
	b, err := experiment.Load(filepath.Join(root, "original"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](filepath.Join(root, "report", "samples.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 30 {
		t.Fatal("native typed export omitted trials", len(rows))
	}
	trials := map[string]experiment.Trial{}
	for _, trial := range b.Trials {
		trials[trial.ID] = trial
	}
	for _, row := range rows {
		trial, known := trials[row.Trial]
		if !known || row.Status != "ok" || row.SnapshotDensityJSON == nil || row.ElapsedNS != nil || row.SampleIndex != nil || row.Operations != nil || row.LatencyEligible {
			t.Fatal("native density row promoted or incomplete", row.Trial)
		}
		var actual experiment.SnapshotDensityTrialEvidence
		if err := json.Unmarshal([]byte(*row.SnapshotDensityJSON), &actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&actual, trial.SnapshotDensity) {
			t.Fatal("native raw evidence changed in Parquet", row.Trial)
		}
	}
}

func TestParquetProcessSnapshotRoundTrip(t *testing.T) {
	// Export preservation is tested with synthetic evidence, not a native
	// qualification. Decimal birth times must never pass through float64.
	proof := &protocol.ProcessSnapshotResult{
		Mode:              protocol.ProcessSnapshotMode,
		Boundary:          protocol.ProcessSnapshotBoundary("process-snapshot-first-write"),
		Source:            protocol.SnapshotProcess{PID: 101, StartTimeTicks: "9007199254740993"},
		Template:          protocol.SnapshotProcess{PID: 102, StartTimeTicks: "9007199254740994"},
		Restored:          protocol.SnapshotProcess{PID: 103, StartTimeTicks: "9007199254740995"},
		AlternateRestored: protocol.SnapshotProcess{PID: 104, StartTimeTicks: "9007199254740996"},
		PreForkThreads:    1, SourceReleased: true, ChildrenReaped: true,
		SourceAfterMutation: 125, RestoredBeforeWrite: 64, RestoredAfterWrite: 64,
		PassiveSegmentProbe: 127, MemoryPages: 3, TableElements: 3,
		IndependentRestorations:     2,
		MemoryAtRestoreSHA256:       protocol.ProcessSnapshotMemorySHA256(false),
		MemoryAfterFirstWriteSHA256: protocol.ProcessSnapshotMemorySHA256(true),
	}
	proof.ClockProcess = proof.Restored
	b := experiment.Bundle{Trials: []experiment.Trial{
		{Status: "ok", Profile: "timing", Scenario: "process-snapshot-first-write", Samples: []protocol.Sample{{Operations: 1, ElapsedNS: 0, Verified: true, ProcessSnapshotResult: proof}}},
		{Status: "unsupported", Reason: "unqualified adapter"},
	}}
	path := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ProcessSnapshotJSON == nil || rows[1].ProcessSnapshotJSON != nil || rows[1].ElapsedNS != nil || rows[1].LatencyEligible || rows[0].ElapsedNS == nil || *rows[0].ElapsedNS != 0 || rows[0].ExportVersion != SampleExportVersion {
		t.Fatal("snapshot evidence or missing/zero semantics lost", rows)
	}
	var actual protocol.ProcessSnapshotResult
	if err := json.Unmarshal([]byte(*rows[0].ProcessSnapshotJSON), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(proof, &actual) {
		t.Fatal("snapshot lineage, clocks or state proof changed")
	}
}

func TestParquetTierWindowRoundTrip(t *testing.T) {
	window := &protocol.TierWindow{Version: 1, ModuleSHA256: "module", Export: "run", Collector: "V8/testing-code-tier-intrinsics", CollectorVersion: "v8-test", Scope: "exported_entry_code_nonatomic_boundary_snapshots", Quality: "engine_reported", Invocation: 1, Before: protocol.TierReading{State: "uncompiled"}, OperationStartNS: 3, OperationEndNS: 13, After: protocol.TierReading{StartNS: 14, EndNS: 15, State: "unavailable", Reason: "non-atomic queries disagreed"}}
	b := experiment.Bundle{Trials: []experiment.Trial{{Profile: "profiling", Scenario: "trajectory", Status: "ok", Samples: []protocol.Sample{{TierWindow: window}}}, {Status: "unsupported"}}}
	path := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].TierWindowJSON == nil || rows[1].TierWindowJSON != nil || rows[0].ExportVersion != SampleExportVersion || rows[0].LatencyEligible {
		t.Fatal("lost diagnostic scope or nullable evidence", rows)
	}
	var actual protocol.TierWindow
	if err := json.Unmarshal([]byte(*rows[0].TierWindowJSON), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(window, &actual) {
		t.Fatal("tier evidence changed in export")
	}
}

func TestParquetContinuationRoundTrip(t *testing.T) {
	proof := &protocol.ContinuationResult{Mode: protocol.NativeContinuationMode, Depth: 128, StackResult: 8263, GuestResumed: true, MemoryAfterRestoreSHA256: protocol.ContinuationMemorySHA256(false), MemoryAfterWriteSHA256: protocol.ContinuationMemorySHA256(true), GlobalAfterRestore: 99, GlobalAfterWrite: 100}
	b := experiment.Bundle{Trials: []experiment.Trial{{Status: "ok", Profile: "timing", Samples: []protocol.Sample{{ContinuationResult: proof}}}, {Status: "unsupported"}}}
	p := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, p); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ContinuationJSON == nil || rows[1].ContinuationJSON != nil || rows[0].ExportVersion != SampleExportVersion {
		t.Fatal("nullable continuation proof lost")
	}
	var actual protocol.ContinuationResult
	if err := json.Unmarshal([]byte(*rows[0].ContinuationJSON), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(proof, &actual) {
		t.Fatal("continuation proof changed in export")
	}
}

func TestParquetTierFailurePrefix(t *testing.T) {
	before := &protocol.TierWindow{Version: 2, Invocation: 1, InvocationOutcome: "returned"}
	failed := &protocol.TierWindow{Version: 2, Invocation: 2, InvocationOutcome: "guest_trap", FailureReason: "unreachable"}
	b := experiment.Bundle{Trials: []experiment.Trial{{Profile: "profiling", Scenario: "trajectory", Status: "error", Reason: "guest call trapped: unreachable", Samples: []protocol.Sample{{Index: 0, Warmup: true, Operations: 1, ElapsedNS: 10, Verified: true, Result: protocol.Values{7}, TierWindow: before}, {Index: 1, Operations: 1, ElapsedNS: 15, Verified: false, TierWindow: failed}}}}}
	path := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].LatencyEligible || rows[1].LatencyEligible || !rows[0].Verified || rows[1].Verified || !rows[0].Warmup || rows[1].Warmup || rows[0].Status != "error" || rows[1].Status != "error" || *rows[1].ElapsedNS != 15 {
		t.Fatal("lost failed diagnostic prefix or promoted successful prefix", rows)
	}
	var actual protocol.TierWindow
	if err := json.Unmarshal([]byte(*rows[1].TierWindowJSON), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(failed, &actual) {
		t.Fatal("lost failure outcome and reason")
	}
}

func TestParquetSustainedClocksAreNullableAndExact(t *testing.T) {
	b := experiment.Bundle{Trials: []experiment.Trial{{Scenario: "steady", Status: "unsupported"}, {Scenario: "sustained", Status: "duration_budget_not_met", Samples: []protocol.Sample{{SustainedWindow: &protocol.SustainedWindow{StartNS: 0, OperationStartNS: 10, OperationEndNS: 20, EndNS: 30}, SustainedRelease: &protocol.SustainedRelease{StartNS: 50, EndNS: 60, Closed: true}}}}}}
	b.Manifest.Lock.Options.SustainedDuration = 1000000
	path := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].SessionWindowStartNS != nil || rows[0].SustainedTargetNS != nil || rows[1].SessionWindowStartNS == nil || *rows[1].SessionWindowStartNS != 0 || *rows[1].SessionOperationEndNS != 20 || *rows[1].SustainedTargetNS != 1000000 || rows[1].Released == nil || !*rows[1].Released || rows[1].LatencyEligible {
		t.Fatal("lost nullable clocks or eligibility", rows)
	}
	if rows[0].ReleasePolicy != nil || rows[1].ReleasePolicy == nil || *rows[1].ReleasePolicy != "runtime_closed" {
		t.Fatal("legacy/default policy lost")
	}
}

func TestParquetJSReferenceRelease(t *testing.T) {
	b := experiment.Bundle{Trials: []experiment.Trial{{Scenario: "sustained", Samples: []protocol.Sample{{SustainedRelease: &protocol.SustainedRelease{Closed: false, Policy: "js_references_dropped"}}}}}}
	p := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, p); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](p)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Released == nil || *rows[0].Released || rows[0].ReleasePolicy == nil || *rows[0].ReleasePolicy != "js_references_dropped" {
		t.Fatal("reference release claimed engine close")
	}
}

func TestParquetPostCollection(t *testing.T) {
	c := &protocol.SustainedCollection{StartNS: 0, EndNS: 100, Policy: "one_forced_go_gc", Observations: []protocol.Observation{{Metric: "host.heap.end", Value: protocol.Value(42), Status: "available", Phase: "sustained/post_collection", Denominator: protocol.SustainedCollectionDenominator}}}
	b := experiment.Bundle{Trials: []experiment.Trial{{Scenario: "sustained", Profile: "memory", Samples: []protocol.Sample{{Operations: 1000, SustainedRelease: &protocol.SustainedRelease{Closed: true, PostCollection: c}}}}, {Scenario: "steady", Status: "unsupported"}}}
	b.Manifest.Lock.Options.SustainedPostCollection = true
	dir := t.TempDir()
	if err := ExportParquet(b, filepath.Join(dir, "samples.parquet")); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](filepath.Join(dir, "samples.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].PostCollectionStartNS == nil || *rows[0].PostCollectionStartNS != 0 || *rows[0].PostCollectionEndNS != 100 || *rows[0].PostCollectionPolicy != c.Policy || !*rows[0].PostCollectionRequested || rows[1].PostCollectionRequested != nil || rows[1].PostCollectionStartNS != nil {
		t.Fatal("lost nullable collection clocks/policy")
	}
	if err := ExportObservations(b, filepath.Join(dir, "observations.parquet")); err != nil {
		t.Fatal(err)
	}
	obs, err := parquet.ReadFile[ObservationRow](filepath.Join(dir, "observations.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].Operations != nil || obs[0].Warmup != nil || obs[0].SampleIndex == nil || obs[0].Phase != "sustained/post_collection" || obs[0].Denominator != protocol.SustainedCollectionDenominator || *obs[0].Value != 42 {
		t.Fatal("collection domain lost or normalized by workload calls", obs)
	}
}

func TestParquetKeepsFailuresAndWarmup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.parquet")
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: "run"}, Trials: []experiment.Trial{{ID: "failed", Block: 0, Status: "unsupported"}, {ID: "ok", Block: 1, Status: "ok", Samples: []protocol.Sample{{Index: 0, Warmup: true, ElapsedNS: 100, Operations: 2, Verified: true}, {Index: 1, ElapsedNS: 50, Operations: 2, Verified: true}}}}}
	if e := ExportParquet(b, path); e != nil {
		t.Fatal(e)
	}
	rows, e := parquet.ReadFile[SampleRow](path)
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 3 || rows[0].ElapsedNS != nil || rows[0].Status != "unsupported" || !rows[1].Warmup || *rows[2].Operations != 2 {
		t.Fatalf("lost measurement semantics: %+v", rows)
	}
}

func TestGuestDensityParquetPreservesEveryInstance(t *testing.T) {
	e := &protocol.GuestDensityResult{Provisioning: "fresh_initialize", Instances: []protocol.GuestDensityInstance{{MemorySHA256: "a", Global: 42, Checksum: 7}, {MemorySHA256: "b", Global: 43, Checksum: 8}}}
	b := experiment.Bundle{Trials: []experiment.Trial{{Samples: []protocol.Sample{{GuestDensityResult: e}}}, {Status: "unsupported"}}}
	path := filepath.Join(t.TempDir(), "samples.parquet")
	if err := ExportParquet(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[SampleRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].GuestDensityJSON == nil || rows[1].GuestDensityJSON != nil {
		t.Fatal("lost nullable guest density evidence")
	}
	if *rows[0].GuestDensityJSON != `{"provisioning":"fresh_initialize","payload_bytes":0,"source_independent":false,"payload_unchanged":false,"instances":[{"memory_sha256":"a","global":42,"checksum":7},{"memory_sha256":"b","global":43,"checksum":8}]}` {
		t.Fatal("lost instance state/policy", *rows[0].GuestDensityJSON)
	}
}

func TestParquetLatencyEligibilityPreservesRawTimers(t *testing.T) {
	for _, profile := range []string{"timing", "memory", "code", "counters", "profiling"} {
		t.Run(profile, func(t *testing.T) {
			b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: profile}}}}
			for i, mode := range []string{"valid", "warmup", "unverified", "negative", "zero_operations", "failed", "sacrificial", "mismatch", "no_samples"} {
				trial := experiment.Trial{ID: mode, Block: i, Status: "ok", Profile: profile, Samples: []protocol.Sample{{Index: 0, ElapsedNS: 50, Operations: 2, Verified: true}}}
				switch mode {
				case "warmup":
					trial.Samples[0].Warmup = true
				case "unverified":
					trial.Samples[0].Verified = false
				case "negative":
					trial.Samples[0].ElapsedNS = -1
				case "zero_operations":
					trial.Samples[0].Operations = 0
				case "failed":
					trial.Status = "incorrect"
				case "sacrificial":
					trial.Block = -1
				case "mismatch":
					trial.Profile = "other"
				case "no_samples":
					trial.Samples = nil
				}
				b.Trials = append(b.Trials, trial)
			}
			path := filepath.Join(t.TempDir(), "samples.parquet")
			if err := ExportParquet(b, path); err != nil {
				t.Fatal(err)
			}
			rows, err := parquet.ReadFile[SampleRow](path)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != len(b.Trials) {
				t.Fatal("dropped evidence")
			}
			for i, row := range rows {
				trial := b.Trials[i]
				want := profile == "timing" && trial.ID == "valid"
				if row.LatencyEligible != want || row.ExportVersion != SampleExportVersion || row.LatencyReason == "" || row.Status != trial.Status || row.Profile != trial.Profile {
					t.Fatalf("wrong eligibility or raw identity: %+v", row)
				}
				if len(trial.Samples) == 0 {
					if row.ElapsedNS != nil {
						t.Fatal("fabricated timer")
					}
				} else if row.ElapsedNS == nil || *row.ElapsedNS != trial.Samples[0].ElapsedNS || row.Operations == nil || *row.Operations != int64(trial.Samples[0].Operations) {
					t.Fatal("raw diagnostic timer changed", row)
				}
			}
		})
	}
}
